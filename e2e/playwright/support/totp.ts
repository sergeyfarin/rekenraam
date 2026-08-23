/**
 * A minimal RFC 6238 TOTP generator, so a browser test can act as the
 * authenticator app the real enrollment flow expects.
 *
 * This deliberately reimplements the algorithm instead of trusting the
 * backend's own code: the point of the journey is that a code produced from
 * the published setup key by an independent implementation is accepted.
 */

import { createHmac } from 'node:crypto';

const BASE32_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
const STEP_SECONDS = 30;
const DIGITS = 6;

function decodeBase32(secret: string): Buffer {
  const normalized = secret.replace(/=+$/, '').replace(/\s+/g, '').toUpperCase();
  let bits = 0;
  let value = 0;
  const bytes: number[] = [];

  for (const character of normalized) {
    const index = BASE32_ALPHABET.indexOf(character);
    if (index < 0) {
      throw new Error(`not a base32 character: ${character}`);
    }
    value = (value << 5) | index;
    bits += 5;
    if (bits >= 8) {
      bits -= 8;
      bytes.push((value >>> bits) & 0xff);
    }
  }

  return Buffer.from(bytes);
}

/** The current TOTP step number, which the caller offsets to dodge the replay guard. */
export function currentStep(now: Date = new Date()): number {
  return Math.floor(now.getTime() / 1000 / STEP_SECONDS);
}

/** The six-digit code for one step of a base32 shared secret. */
export function totpCode(secret: string, step: number): string {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(step));

  const digest = createHmac('sha1', decodeBase32(secret)).update(counter).digest();
  const offset = digest[digest.length - 1] & 0x0f;
  const truncated =
    ((digest[offset] & 0x7f) << 24) |
    ((digest[offset + 1] & 0xff) << 16) |
    ((digest[offset + 2] & 0xff) << 8) |
    (digest[offset + 3] & 0xff);

  return String(truncated % 10 ** DIGITS).padStart(DIGITS, '0');
}

/** Pulls the shared secret out of an `otpauth://` setup link. */
export function secretFromOTPAuthURI(uri: string): string {
  const match = /[?&]secret=([A-Z2-7]+)/i.exec(uri);
  if (!match) {
    throw new Error(`no secret in otpauth uri: ${uri}`);
  }

  return match[1];
}
