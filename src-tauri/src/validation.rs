/// Input validation for Rekenraam domain types.
///
/// All validators return `Result<(), AppError>` with `AppError::Validation` so that
/// the frontend can show field-level error messages.

use crate::error::AppError;

// ── Account types ─────────────────────────────────────────────────────────────
// Must match the CHECK constraint in V1__init.sql accounts.type
pub const ACCOUNT_TYPES: &[&str] = &[
    "cash", "checking", "savings", "credit", "loan",
    "investment", "asset", "liability", "income", "expense", "equity",
];

/// Validate that `t` is a recognised account type.
pub fn validate_account_type(t: &str) -> Result<(), AppError> {
    if ACCOUNT_TYPES.contains(&t) {
        Ok(())
    } else {
        Err(AppError::validation(
            "account_type",
            format!("invalid account_type '{}'; must be one of: {}", t, ACCOUNT_TYPES.join(", ")),
        ))
    }
}

// ── Transaction statuses ───────────────────────────────────────────────────────
// Must match the CHECK constraint in V1__init.sql transactions.status
pub const TX_STATUSES: &[&str] = &["uncleared", "cleared", "reconciled", "void"];

/// Validate that `s` is a recognised transaction status.
pub fn validate_tx_status(s: &str) -> Result<(), AppError> {
    if TX_STATUSES.contains(&s) {
        Ok(())
    } else {
        Err(AppError::validation(
            "status",
            format!("invalid status '{}'; must be one of: {}", s, TX_STATUSES.join(", ")),
        ))
    }
}

// ── Name / text length limits ─────────────────────────────────────────────────
pub const MAX_NAME_LEN: usize = 512;
pub const MAX_MEMO_LEN: usize = 4096;

/// Validate that `name` is non-empty and within the maximum length.
pub fn validate_name(name: &str) -> Result<(), AppError> {
    let trimmed = name.trim();
    if trimmed.is_empty() {
        return Err(AppError::validation("name", "name must not be blank"));
    }
    if trimmed.len() > MAX_NAME_LEN {
        return Err(AppError::validation(
            "name",
            format!("name must not exceed {} characters (got {})", MAX_NAME_LEN, trimmed.len()),
        ));
    }
    Ok(())
}

/// Validate that `memo` is within the maximum length (empty is allowed).
pub fn validate_memo(memo: &str) -> Result<(), AppError> {
    if memo.len() > MAX_MEMO_LEN {
        return Err(AppError::validation(
            "memo",
            format!("memo must not exceed {} characters (got {})", MAX_MEMO_LEN, memo.len()),
        ));
    }
    Ok(())
}

// ── Amount bounds ─────────────────────────────────────────────────────────────
// Guard against values that could overflow i64 arithmetic in balance calculations.
// Individual splits are capped at ±10 billion in the base unit (e.g. 10,000,000,000 cents).
pub const MAX_AMOUNT_MINOR: i64 = 1_000_000_000_000_00; // 10 billion in minor units (2dp)

/// Validate that `amount` is within acceptable bounds.
pub fn validate_amount_minor(amount: i64) -> Result<(), AppError> {
    // Use saturating_abs so i64::MIN (which has no positive i64 counterpart) is
    // treated as i64::MAX, which is guaranteed to exceed MAX_AMOUNT_MINOR.
    if amount.saturating_abs() > MAX_AMOUNT_MINOR {
        return Err(AppError::validation(
            "amount_minor",
            format!("amount_minor {} exceeds the maximum of ±{} (overflow guard)", amount, MAX_AMOUNT_MINOR),
        ));
    }
    Ok(())
}

// ── LIKE pattern escaping ─────────────────────────────────────────────────────

/// Escape `%`, `_`, and `\` in a user-supplied string for use in a SQLite LIKE
/// pattern with `ESCAPE '\'`.
///
/// Without this, a search for `%` matches every row, and `_` matches any single
/// character — producing incorrect results (not an injection risk since params
/// are bound, but logically wrong).
pub fn escape_like(input: &str) -> String {
    let mut out = String::with_capacity(input.len() + 4);
    for ch in input.chars() {
        match ch {
            '\\' => out.push_str(r"\\"),
            '%'  => out.push_str(r"\%"),
            '_'  => out.push_str(r"\_"),
            _    => out.push(ch),
        }
    }
    out
}

// ── Tests ─────────────────────────────────────────────────────────────────────
#[cfg(test)]
mod tests {
    use super::*;

    // account_type ─────────────────────────────────────────────────────────────

    #[test]
    fn test_validate_account_type_valid() {
        for t in ACCOUNT_TYPES {
            assert!(validate_account_type(t).is_ok(), "expected ok for '{}'", t);
        }
    }

    #[test]
    fn test_validate_account_type_invalid() {
        assert!(validate_account_type("epxense").is_err());
        assert!(validate_account_type("").is_err());
        assert!(validate_account_type("CHECKING").is_err()); // case-sensitive
        assert!(validate_account_type("bank").is_err());
    }

    // tx_status ────────────────────────────────────────────────────────────────

    #[test]
    fn test_validate_tx_status_valid() {
        for s in TX_STATUSES {
            assert!(validate_tx_status(s).is_ok(), "expected ok for '{}'", s);
        }
    }

    #[test]
    fn test_validate_tx_status_invalid() {
        assert!(validate_tx_status("pending").is_err());
        assert!(validate_tx_status("Void").is_err()); // case-sensitive
        assert!(validate_tx_status("").is_err());
        assert!(validate_tx_status("unknown").is_err());
    }

    // name ─────────────────────────────────────────────────────────────────────

    #[test]
    fn test_validate_name_valid() {
        assert!(validate_name("My Account").is_ok());
        assert!(validate_name("a").is_ok());
    }

    #[test]
    fn test_validate_name_blank() {
        assert!(validate_name("").is_err());
        assert!(validate_name("   ").is_err());
    }

    #[test]
    fn test_validate_name_too_long() {
        let long = "x".repeat(MAX_NAME_LEN + 1);
        assert!(validate_name(&long).is_err());
        let ok = "x".repeat(MAX_NAME_LEN);
        assert!(validate_name(&ok).is_ok());
    }

    // memo ─────────────────────────────────────────────────────────────────────

    #[test]
    fn test_validate_memo_empty_ok() {
        assert!(validate_memo("").is_ok());
    }

    #[test]
    fn test_validate_memo_too_long() {
        let long = "x".repeat(MAX_MEMO_LEN + 1);
        assert!(validate_memo(&long).is_err());
        let ok = "x".repeat(MAX_MEMO_LEN);
        assert!(validate_memo(&ok).is_ok());
    }

    // amount_minor ─────────────────────────────────────────────────────────────

    #[test]
    fn test_validate_amount_minor_valid() {
        assert!(validate_amount_minor(0).is_ok());
        assert!(validate_amount_minor(10_000).is_ok());
        assert!(validate_amount_minor(-10_000).is_ok());
        assert!(validate_amount_minor(MAX_AMOUNT_MINOR).is_ok());
        assert!(validate_amount_minor(-MAX_AMOUNT_MINOR).is_ok());
    }

    #[test]
    fn test_validate_amount_minor_overflow() {
        assert!(validate_amount_minor(MAX_AMOUNT_MINOR + 1).is_err());
        assert!(validate_amount_minor(i64::MAX).is_err());
        assert!(validate_amount_minor(i64::MIN).is_err());
    }

    // escape_like ──────────────────────────────────────────────────────────────

    #[test]
    fn test_escape_like_no_special_chars() {
        assert_eq!(escape_like("hello"), "hello");
    }

    #[test]
    fn test_escape_like_percent() {
        assert_eq!(escape_like("%"), r"\%");
        assert_eq!(escape_like("50%"), r"50\%");
    }

    #[test]
    fn test_escape_like_underscore() {
        assert_eq!(escape_like("a_b"), r"a\_b");
    }

    #[test]
    fn test_escape_like_backslash() {
        assert_eq!(escape_like(r"a\b"), r"a\\b");
    }

    #[test]
    fn test_escape_like_all_special() {
        // Input: '\', '_', '%' (3 chars)
        // '\' → '\\', '_' → '\_', '%' → '\%'
        // Result: '\\', '\', '_', '\', '%' = \\\_\%
        assert_eq!(escape_like(r"\_%"), r"\\\_\%");
    }
}
