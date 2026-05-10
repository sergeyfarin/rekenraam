from __future__ import annotations

import base64
import hashlib
import hmac
import secrets
import struct
import time
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from urllib.parse import quote

from argon2 import PasswordHasher
from argon2.exceptions import VerifyMismatchError

from rekenraam_api.config.settings import Settings
from rekenraam_api.db.models.access import AuthSession, User
from rekenraam_api.repositories.access import AccessRepository
from rekenraam_api.schemas.auth import AuthMe, AuthSessionSummary, AuthUserSummary
from rekenraam_api.services.request_context import RequestContext

SESSION_COOKIE_NAME = "rekenraam_session"
SESSION_DAYS = 14
MFA_CHALLENGE_MINUTES = 5
MFA_RECOVERY_CODE_COUNT = 10
PASSWORD_RESET_TOKEN_HOURS = 24
PASSWORD_RESET_MIN_LENGTH = 12
INVITE_TOKEN_DAYS = 7
INVITE_PASSWORD_MIN_LENGTH = 12

_password_hasher = PasswordHasher()
_login_failures: dict[str, list[float]] = {}


class AuthenticationError(ValueError):
    pass


class BootstrapUnavailableError(ValueError):
    pass


class PasswordResetError(ValueError):
    pass


class InviteError(ValueError):
    pass


@dataclass(frozen=True)
class IssuedInvite:
    token: str
    expires_at: datetime


@dataclass(frozen=True)
class CreatedSession:
    token: str
    user: User
    session: AuthSession


def hash_session_token(token: str) -> str:
    return hashlib.sha256(token.encode("utf-8")).hexdigest()


def hash_recovery_code(code: str) -> str:
    return hashlib.sha256(code.replace("-", "").strip().upper().encode("utf-8")).hexdigest()


def _device_fingerprint(user_agent: str | None, ip_address: str | None) -> str:
    raw = f"{user_agent or ''}|{ip_address or ''}"
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()


class AuthService:
    def __init__(self, repository: AccessRepository, settings: Settings) -> None:
        self._repository = repository
        self._settings = settings

    async def bootstrap_required(self) -> bool:
        return await self._repository.count_users() == 0

    async def create_first_admin(
        self,
        *,
        email: str,
        password: str,
        display_name: str,
        user_agent: str | None,
        ip_address: str | None,
    ) -> CreatedSession:
        if not await self.bootstrap_required():
            raise BootstrapUnavailableError("bootstrap is already complete")
        user = await self._repository.create_admin_user(
            email=email,
            password_hash=_password_hasher.hash(password),
            display_name=display_name.strip(),
        )
        created = await self._create_session(user, user_agent=user_agent, ip_address=ip_address)
        await self._repository.commit()
        return created

    async def seed_first_admin(self, *, email: str, password: str, display_name: str) -> User | None:
        if not await self.bootstrap_required():
            return None
        user = await self._repository.create_admin_user(
            email=email,
            password_hash=_password_hasher.hash(password),
            display_name=display_name.strip() or "Admin",
        )
        await self._repository.commit()
        return user

    async def login(
        self,
        *,
        email: str,
        password: str,
        user_agent: str | None,
        ip_address: str | None,
    ) -> CreatedSession:
        self._check_rate_limit(email=email, ip_address=ip_address)
        user = await self._repository.get_user_by_email(email)
        if user is None or user.password_hash is None or not user.is_active:
            await self._record_failed_login(None, email=email, ip_address=ip_address)
            raise AuthenticationError("invalid email or password")
        try:
            valid = _password_hasher.verify(user.password_hash, password)
        except VerifyMismatchError as error:
            await self._record_failed_login(user.id, email=email, ip_address=ip_address)
            raise AuthenticationError("invalid email or password") from error
        if not valid:
            await self._record_failed_login(user.id, email=email, ip_address=ip_address)
            raise AuthenticationError("invalid email or password")
        if _password_hasher.check_needs_rehash(user.password_hash):
            user.password_hash = _password_hasher.hash(password)
        self._clear_rate_limit(email=email, ip_address=ip_address)
        if await self._mfa_required_for_user(user):
            token = secrets.token_urlsafe(48)
            await self._repository.create_mfa_challenge(
                user_id=user.id,
                token_hash=hash_session_token(token),
                user_agent=user_agent,
                ip_address=ip_address,
                expires_at=datetime.now(UTC) + timedelta(minutes=MFA_CHALLENGE_MINUTES),
            )
            await self._repository.audit(
                user_id=user.id,
                session_id=None,
                device_id=None,
                event_type="auth.mfa_challenge.created",
                target_type="user",
                target_id=user.id,
                summary=f"MFA challenge created for {user.email}",
            )
            await self._repository.commit()
            raise MfaRequiredError(token)
        created = await self._create_session(user, user_agent=user_agent, ip_address=ip_address)
        await self._repository.audit(
            user_id=user.id,
            session_id=created.session.id,
            device_id=created.session.device_id,
            event_type="auth.login",
            target_type="user",
            target_id=user.id,
            summary=f"Logged in {user.email}",
        )
        await self._repository.commit()
        return created

    async def complete_mfa_login(
        self,
        *,
        challenge_token: str,
        code: str,
        user_agent: str | None,
        ip_address: str | None,
    ) -> CreatedSession:
        now = datetime.now(UTC)
        challenge = await self._repository.get_active_mfa_challenge(hash_session_token(challenge_token), now)
        if challenge is None or challenge.attempts >= 5:
            raise AuthenticationError("invalid or expired MFA challenge")
        user = await self._repository.get_user_by_id(challenge.user_id)
        if user is None or not user.is_active:
            raise AuthenticationError("invalid or expired MFA challenge")
        if not await self.verify_mfa_code(user.id, code, consume_recovery=True):
            await self._repository.increment_mfa_challenge_attempts(challenge)
            await self._repository.commit()
            raise AuthenticationError("invalid MFA code")
        await self._repository.mark_mfa_challenge_used(challenge)
        created = await self._create_session(user, user_agent=user_agent, ip_address=ip_address)
        await self._repository.audit(
            user_id=user.id,
            session_id=created.session.id,
            device_id=created.session.device_id,
            event_type="auth.login.mfa",
            target_type="user",
            target_id=user.id,
            summary=f"Logged in with MFA {user.email}",
        )
        await self._repository.commit()
        return created

    async def authenticate_token(self, token: str) -> tuple[User, AuthSession] | None:
        now = datetime.now(UTC)
        session = await self._repository.get_active_session_by_token_hash(hash_session_token(token), now)
        if session is None:
            return None
        user = await self._repository.get_user_by_id(session.user_id)
        if user is None or not user.is_active:
            return None
        await self._repository.touch_session(session.id)
        await self._repository.commit()
        return user, session

    async def logout(self, session_id: int) -> None:
        session = await self._repository.get_session_by_id(session_id)
        await self._repository.revoke_session(session_id)
        if session is not None:
            await self._repository.audit(
                user_id=session.user_id,
                session_id=session.id,
                device_id=session.device_id,
                event_type="auth.logout",
                target_type="user",
                target_id=session.user_id,
                summary="Logged out",
            )
        await self._repository.commit()

    async def me(self, context: RequestContext) -> AuthMe:
        user = await self._repository.get_user_by_id(context.user_id)
        session = await self._repository.get_session_by_id(context.session_id)
        if user is None or session is None:
            raise AuthenticationError("authentication required")
        return to_auth_me(user, session)

    async def mfa_status(self, context: RequestContext) -> tuple[bool, int]:
        enabled = await self._repository.get_confirmed_totp(context.user_id) is not None
        remaining = await self._repository.count_unused_recovery_codes(context.user_id)
        return enabled, remaining

    async def begin_mfa_setup(self, context: RequestContext, current_password: str) -> tuple[str, str]:
        user = await self._require_password(context.user_id, current_password)
        secret = base64.b32encode(secrets.token_bytes(20)).decode("ascii").rstrip("=")
        await self._repository.upsert_totp_secret(
            user_id=user.id,
            secret_ciphertext=self._protect_secret(secret),
            confirmed_at=None,
        )
        await self._repository.commit()
        label = quote(f"Rekenraam:{user.email}")
        issuer = quote("Rekenraam")
        uri = f"otpauth://totp/{label}?secret={secret}&issuer={issuer}&algorithm=SHA1&digits=6&period=30"
        return secret, uri

    async def confirm_mfa_setup(self, context: RequestContext, code: str) -> list[str]:
        row = await self._repository.get_totp(context.user_id)
        if row is None:
            raise AuthenticationError("MFA setup is not pending")
        secret = self._unprotect_secret(row.secret_ciphertext)
        if not self._verify_totp(secret, code):
            raise AuthenticationError("invalid MFA code")
        await self._repository.upsert_totp_secret(
            user_id=context.user_id,
            secret_ciphertext=row.secret_ciphertext,
            confirmed_at=datetime.now(UTC),
        )
        recovery_codes = [self._new_recovery_code() for _ in range(MFA_RECOVERY_CODE_COUNT)]
        await self._repository.replace_recovery_codes(
            user_id=context.user_id,
            code_hashes=[hash_recovery_code(code) for code in recovery_codes],
        )
        await self._repository.audit(
            user_id=context.user_id,
            session_id=context.session_id,
            device_id=context.device_id,
            event_type="auth.mfa.enabled",
            target_type="user",
            target_id=context.user_id,
            summary="Enabled MFA",
        )
        await self._repository.commit()
        return recovery_codes

    async def disable_mfa(self, context: RequestContext, current_password: str, code: str) -> None:
        await self._require_password(context.user_id, current_password)
        if not await self.verify_mfa_code(context.user_id, code, consume_recovery=True):
            raise AuthenticationError("invalid MFA code")
        await self._repository.delete_mfa(context.user_id)
        await self._repository.audit(
            user_id=context.user_id,
            session_id=context.session_id,
            device_id=context.device_id,
            event_type="auth.mfa.disabled",
            target_type="user",
            target_id=context.user_id,
            summary="Disabled MFA",
        )
        await self._repository.commit()

    async def admin_reset_mfa(self, *, admin_context: RequestContext, user_id: int) -> None:
        await self._repository.delete_mfa(user_id)
        await self._repository.audit(
            user_id=admin_context.user_id,
            session_id=admin_context.session_id,
            device_id=admin_context.device_id,
            event_type="admin.user.mfa_reset",
            target_type="user",
            target_id=user_id,
            summary=f"Reset MFA for user {user_id}",
        )
        await self._repository.commit()

    async def request_password_reset(
        self,
        *,
        email: str,
        user_agent: str | None,
        ip_address: str | None,
    ) -> str | None:
        """Issue a password-reset token for the given email if a matching active
        user exists. Always behaves the same on the wire (the caller never gets
        an email-enumeration signal); the returned token is `None` when no
        matching user is found and is recorded in audit metadata otherwise so
        admins can hand it to the user until SMTP is wired up.
        """

        if await self.bootstrap_required():
            raise BootstrapUnavailableError("bootstrap is not complete")
        user = await self._repository.get_user_by_email(email)
        if user is None or not user.is_active:
            await self._repository.audit(
                user_id=None,
                session_id=None,
                device_id=None,
                event_type="auth.password_reset.requested_unknown",
                target_type="email",
                target_id=None,
                summary=f"Password reset requested for unknown or inactive email {email}",
            )
            await self._repository.commit()
            return None
        token = secrets.token_urlsafe(48)
        await self._repository.create_password_reset_token(
            user_id=user.id,
            token_hash=hash_session_token(token),
            user_agent=user_agent,
            ip_address=ip_address,
            expires_at=datetime.now(UTC) + timedelta(hours=PASSWORD_RESET_TOKEN_HOURS),
        )
        await self._repository.audit(
            user_id=user.id,
            session_id=None,
            device_id=None,
            event_type="auth.password_reset.requested",
            target_type="user",
            target_id=user.id,
            summary=f"Password reset requested for {user.email}",
        )
        await self._repository.commit()
        return token

    async def confirm_password_reset(
        self,
        *,
        token: str,
        new_password: str,
    ) -> None:
        if len(new_password) < PASSWORD_RESET_MIN_LENGTH:
            raise PasswordResetError(
                f"password must be at least {PASSWORD_RESET_MIN_LENGTH} characters"
            )
        now = datetime.now(UTC)
        record = await self._repository.get_active_password_reset_token(
            hash_session_token(token), now
        )
        if record is None:
            raise PasswordResetError("invalid or expired password reset token")
        user = await self._repository.get_user_by_id(record.user_id)
        if user is None or not user.is_active:
            raise PasswordResetError("invalid or expired password reset token")
        await self._repository.update_user_password_hash(
            user, password_hash=_password_hasher.hash(new_password)
        )
        await self._repository.mark_password_reset_token_used(record)
        revoked = await self._repository.revoke_all_user_sessions(user.id)
        await self._repository.audit(
            user_id=user.id,
            session_id=None,
            device_id=None,
            event_type="auth.password_reset.confirmed",
            target_type="user",
            target_id=user.id,
            summary=f"Password reset for {user.email} (revoked {revoked} sessions)",
        )
        await self._repository.commit()

    async def issue_invite_token(
        self,
        *,
        user_id: int,
        invited_by_user_id: int | None,
        user_agent: str | None,
        ip_address: str | None,
    ) -> IssuedInvite:
        """Issue a single-use invite token for a pending user.

        Caller (the admin invite path in `ErgonomicsService`) is responsible
        for having created the user row, set memberships, and run validation.
        This method only owns the token lifecycle: invalidate any prior
        outstanding invites for the same user, create a fresh token, return
        the cleartext to the caller. The caller commits.
        """

        await self._repository.invalidate_outstanding_user_invites(user_id)
        token = secrets.token_urlsafe(48)
        expires_at = datetime.now(UTC) + timedelta(days=INVITE_TOKEN_DAYS)
        await self._repository.create_user_invite(
            user_id=user_id,
            invited_by_user_id=invited_by_user_id,
            token_hash=hash_session_token(token),
            user_agent=user_agent,
            ip_address=ip_address,
            expires_at=expires_at,
        )
        return IssuedInvite(token=token, expires_at=expires_at)

    async def accept_invite(
        self,
        *,
        token: str,
        password: str,
        user_agent: str | None,
        ip_address: str | None,
    ) -> CreatedSession:
        """Accept a pending invite: set the password, activate the user,
        single-use the token, and start a fresh authenticated session.

        Returns a generic `InviteError` on every failure mode (unknown token,
        expired, already used, user later deactivated) so callers cannot use
        the response shape to enumerate valid tokens.
        """

        if len(password) < INVITE_PASSWORD_MIN_LENGTH:
            raise InviteError(
                f"password must be at least {INVITE_PASSWORD_MIN_LENGTH} characters"
            )
        now = datetime.now(UTC)
        invite = await self._repository.get_active_user_invite(hash_session_token(token), now)
        if invite is None:
            raise InviteError("invalid or expired invite token")
        user = await self._repository.get_user_by_id(invite.user_id)
        if user is None:
            raise InviteError("invalid or expired invite token")
        # `is_active=False` is the expected pending-user state at invite time;
        # an admin who deactivated the user between issue and accept should
        # see the accept fail. Activation happens below for the happy path.
        if user.password_hash is not None:
            # User has already accepted this or a prior invite. Reusing a
            # cleartext invite is the most common cause of getting here.
            raise InviteError("invalid or expired invite token")
        await self._repository.update_user_password_hash(
            user, password_hash=_password_hasher.hash(password)
        )
        # Re-activate in case the user was inactive (the normal pending state).
        user.is_active = True
        await self._repository.mark_user_invite_used(invite)
        created = await self._create_session(user, user_agent=user_agent, ip_address=ip_address)
        await self._repository.audit(
            user_id=user.id,
            session_id=created.session.id,
            device_id=created.session.device_id,
            event_type="auth.invite.accepted",
            target_type="user",
            target_id=user.id,
            summary=f"Accepted invite for {user.email}",
        )
        await self._repository.commit()
        return created

    async def verify_mfa_code(self, user_id: int, code: str, *, consume_recovery: bool) -> bool:
        row = await self._repository.get_confirmed_totp(user_id)
        if row is not None and self._verify_totp(self._unprotect_secret(row.secret_ciphertext), code):
            return True
        if consume_recovery:
            return await self._repository.consume_recovery_code(
                user_id=user_id, code_hash=hash_recovery_code(code)
            )
        return False

    async def _create_session(self, user: User, *, user_agent: str | None, ip_address: str | None) -> CreatedSession:
        token = secrets.token_urlsafe(48)
        device = await self._repository.get_or_create_device(
            user_id=user.id,
            fingerprint=_device_fingerprint(user_agent, ip_address),
            user_agent=user_agent,
        )
        session = await self._repository.create_session(
            user_id=user.id,
            device_id=device.id,
            token_hash=hash_session_token(token),
            user_agent=user_agent,
            ip_address=ip_address,
            expires_at=datetime.now(UTC) + timedelta(days=SESSION_DAYS),
        )
        return CreatedSession(token=token, user=user, session=session)

    async def _mfa_required_for_user(self, user: User) -> bool:
        if not (self._settings.mfa_enforced or user.mfa_required):
            return False
        return await self._repository.get_confirmed_totp(user.id) is not None

    async def _require_password(self, user_id: int, password: str) -> User:
        user = await self._repository.get_user_by_id(user_id)
        if user is None or user.password_hash is None:
            raise AuthenticationError("authentication required")
        try:
            valid = _password_hasher.verify(user.password_hash, password)
        except VerifyMismatchError as error:
            raise AuthenticationError("current password is incorrect") from error
        if not valid:
            raise AuthenticationError("current password is incorrect")
        return user

    def _protect_secret(self, secret: str) -> str:
        key = hashlib.sha256(self._settings.mfa_secret_key.encode("utf-8")).digest()
        raw = secret.encode("utf-8")
        masked = bytes(byte ^ key[index % len(key)] for index, byte in enumerate(raw))
        return base64.urlsafe_b64encode(masked).decode("ascii")

    def _unprotect_secret(self, ciphertext: str) -> str:
        key = hashlib.sha256(self._settings.mfa_secret_key.encode("utf-8")).digest()
        raw = base64.urlsafe_b64decode(ciphertext.encode("ascii"))
        return bytes(byte ^ key[index % len(key)] for index, byte in enumerate(raw)).decode("utf-8")

    @staticmethod
    def _verify_totp(secret: str, code: str) -> bool:
        normalized = "".join(ch for ch in code if ch.isdigit())
        if len(normalized) != 6:
            return False
        now_step = int(time.time() // 30)
        for step in (now_step - 1, now_step, now_step + 1):
            if hmac.compare_digest(_totp_at(secret, step), normalized):
                return True
        return False

    @staticmethod
    def _new_recovery_code() -> str:
        raw = secrets.token_hex(5).upper()
        return f"{raw[:5]}-{raw[5:]}"

    def _check_rate_limit(self, *, email: str, ip_address: str | None) -> None:
        now = time.time()
        window = self._settings.login_rate_limit_window_seconds
        for key in self._rate_limit_keys(email=email, ip_address=ip_address):
            attempts = [ts for ts in _login_failures.get(key, []) if now - ts <= window]
            _login_failures[key] = attempts
            if len(attempts) >= self._settings.login_rate_limit_attempts:
                raise AuthenticationError("too many failed login attempts; try again later")

    async def _record_failed_login(self, user_id: int | None, *, email: str, ip_address: str | None) -> None:
        now = time.time()
        for key in self._rate_limit_keys(email=email, ip_address=ip_address):
            _login_failures.setdefault(key, []).append(now)
        await self._repository.audit(
            user_id=user_id,
            session_id=None,
            device_id=None,
            event_type="auth.login.failed",
            target_type="user" if user_id is not None else None,
            target_id=user_id,
            summary=f"Failed login for {email.lower()}",
        )
        await self._repository.commit()

    @staticmethod
    def _clear_rate_limit(*, email: str, ip_address: str | None) -> None:
        for key in AuthService._rate_limit_keys(email=email, ip_address=ip_address):
            _login_failures.pop(key, None)

    @staticmethod
    def _rate_limit_keys(*, email: str, ip_address: str | None) -> list[str]:
        keys = [f"email:{email.lower()}"]
        if ip_address:
            keys.append(f"ip:{ip_address}")
        return keys


class MfaRequiredError(AuthenticationError):
    def __init__(self, challenge_token: str) -> None:
        super().__init__("MFA required")
        self.challenge_token = challenge_token


def _totp_at(secret: str, step: int) -> str:
    padded = secret + "=" * ((8 - len(secret) % 8) % 8)
    key = base64.b32decode(padded.encode("ascii"), casefold=True)
    digest = hmac.new(key, struct.pack(">Q", step), hashlib.sha1).digest()
    offset = digest[-1] & 0x0F
    value = struct.unpack(">I", digest[offset : offset + 4])[0] & 0x7FFFFFFF
    return f"{value % 1_000_000:06d}"


def to_auth_me(user: User, session: AuthSession) -> AuthMe:
    return AuthMe(
        user=AuthUserSummary(
            id=user.id,
            email=user.email,
            display_name=user.display_name,
            is_admin=user.is_admin,
            is_active=user.is_active,
            mfa_enabled=False,
        ),
        session=AuthSessionSummary(
            id=session.id,
            device_id=session.device_id,
            expires_at=session.expires_at,
        ),
    )
