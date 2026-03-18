use async_trait::async_trait;
use chrono::{DateTime, NaiveDate, TimeZone, Utc};
use serde::Deserialize;
use std::collections::HashMap;
use std::time::Duration;

#[derive(Clone, Debug)]
pub struct FxRatePoint {
    pub date: String,
    pub base: String,
    pub quote: String,
    pub rate: f64,
    pub source_name: String,
    #[allow(dead_code)]
    pub source_url: Option<String>,
    pub is_derived: bool,
    pub derived_via: Option<String>,
}

#[derive(Clone, Debug)]
pub struct ProviderRequest {
    pub base: String,
    pub symbols: Vec<String>,
    pub start_date: String,
    pub end_date: String,
}

#[async_trait]
pub trait FxRateProvider: Send + Sync {
    fn name(&self) -> &str;
    async fn fetch_daily_rates(&self, req: ProviderRequest) -> Result<Vec<FxRatePoint>, String>;
}

/// Shared HTTP GET with a 30-second timeout. All providers must use this
/// instead of `reqwest::get` so that hung network requests don't block the
/// FX scheduler indefinitely.
async fn http_get(url: &str) -> Result<reqwest::Response, String> {
    reqwest::Client::builder()
        .timeout(Duration::from_secs(30))
        .build()
        .map_err(|e| e.to_string())?
        .get(url)
        .send()
        .await
        .map_err(|e| e.to_string())
}

fn map_rates_for_dates(
    base: &str,
    rates_by_date: HashMap<String, HashMap<String, f64>>,
    source_name: &str,
    source_url: Option<String>,
) -> Vec<FxRatePoint> {
    let mut out = Vec::new();
    for (date, rates) in rates_by_date {
        for (quote, rate) in rates {
            out.push(FxRatePoint {
                date: date.clone(),
                base: base.to_string(),
                quote,
                rate,
                source_name: source_name.to_string(),
                source_url: source_url.clone(),
                is_derived: false,
                derived_via: None,
            });
        }
    }
    out
}

// ============================================================================
// Frankfurter (ECB)
// ============================================================================

pub struct FrankfurterProvider {
    pub base_url: String,
}

impl Default for FrankfurterProvider {
    fn default() -> Self {
        Self {
            base_url: "https://api.frankfurter.app".to_string(),
        }
    }
}

#[derive(Deserialize)]
struct FrankfurterResponse {
    rates: HashMap<String, HashMap<String, f64>>,
    base: String,
}

#[async_trait]
impl FxRateProvider for FrankfurterProvider {
    fn name(&self) -> &str {
        "ECB/Frankfurter"
    }

    async fn fetch_daily_rates(&self, req: ProviderRequest) -> Result<Vec<FxRatePoint>, String> {
        let symbols = req.symbols.join(",");
        let url = format!(
            "{}/{}..{}?from={}&to={}",
            self.base_url, req.start_date, req.end_date, req.base, symbols
        );

        let resp = http_get(&url).await?;
        if !resp.status().is_success() {
            return Err(format!("Frankfurter HTTP {}", resp.status()));
        }
        let payload: FrankfurterResponse = resp.json().await.map_err(|e| e.to_string())?;
        Ok(map_rates_for_dates(
            &payload.base,
            payload.rates,
            self.name(),
            Some(url),
        ))
    }
}

// ============================================================================
// ExchangeRate.host
// ============================================================================

pub struct ExchangeRateHostProvider;

#[derive(Deserialize)]
struct ExchangeRateHostResponse {
    rates: HashMap<String, HashMap<String, f64>>,
    base: String,
}

#[async_trait]
impl FxRateProvider for ExchangeRateHostProvider {
    fn name(&self) -> &str {
        "ExchangeRate.host"
    }

    async fn fetch_daily_rates(&self, req: ProviderRequest) -> Result<Vec<FxRatePoint>, String> {
        let symbols = req.symbols.join(",");
        let url = format!(
            "https://api.exchangerate.host/timeseries?base={}&symbols={}&start_date={}&end_date={}",
            req.base, symbols, req.start_date, req.end_date
        );
        let resp = http_get(&url).await?;
        if !resp.status().is_success() {
            return Err(format!("ExchangeRate.host HTTP {}", resp.status()));
        }
        let payload: ExchangeRateHostResponse = resp.json().await.map_err(|e| e.to_string())?;
        Ok(map_rates_for_dates(
            &payload.base,
            payload.rates,
            self.name(),
            Some(url),
        ))
    }
}

// ============================================================================
// Yahoo Finance (chart API)
// ============================================================================

pub struct YahooProvider;

#[async_trait]
impl FxRateProvider for YahooProvider {
    fn name(&self) -> &str {
        "Yahoo Finance"
    }

    async fn fetch_daily_rates(&self, req: ProviderRequest) -> Result<Vec<FxRatePoint>, String> {
        let mut out = Vec::new();
        let start = parse_date_to_epoch(&req.start_date)?;
        let end = parse_date_to_epoch(&req.end_date)?;

        for quote in req.symbols.iter() {
            let ticker = format!("{}{}=X", req.base, quote);
            let url = format!(
                "https://query1.finance.yahoo.com/v8/finance/chart/{}?period1={}&period2={}&interval=1d&events=history&includeAdjustedClose=true",
                ticker, start, end
            );

            let resp = http_get(&url).await?;
            if !resp.status().is_success() {
                return Err(format!("Yahoo HTTP {}", resp.status()));
            }

            let payload: serde_json::Value = resp.json().await.map_err(|e| e.to_string())?;
            let result = payload["chart"]["result"].get(0).ok_or_else(|| "Yahoo missing result".to_string())?;
            let timestamps = result["timestamp"].as_array().ok_or_else(|| "Yahoo missing timestamps".to_string())?;
            let closes = result["indicators"]["quote"].get(0)
                .and_then(|v| v["close"].as_array())
                .ok_or_else(|| "Yahoo missing closes".to_string())?;

            for (idx, ts) in timestamps.iter().enumerate() {
                let ts_i = ts.as_i64().ok_or_else(|| "Yahoo invalid timestamp".to_string())?;
                let close = closes
                    .get(idx)
                    .and_then(|v| v.as_f64())
                    .unwrap_or(0.0);
                if close <= 0.0 {
                    continue;
                }
                let date = epoch_to_date(ts_i)?;
                out.push(FxRatePoint {
                    date,
                    base: req.base.clone(),
                    quote: quote.to_string(),
                    rate: close,
                    source_name: self.name().to_string(),
                    source_url: Some(url.clone()),
                    is_derived: false,
                    derived_via: None,
                });
            }
        }

        Ok(out)
    }
}

fn parse_date_to_epoch(date: &str) -> Result<i64, String> {
    let nd = NaiveDate::parse_from_str(date, "%Y-%m-%d").map_err(|e| e.to_string())?;
    Ok(Utc.from_utc_datetime(&nd.and_hms_opt(0, 0, 0).ok_or("invalid time")?).timestamp())
}

fn epoch_to_date(epoch: i64) -> Result<String, String> {
    let dt: DateTime<Utc> = Utc.timestamp_opt(epoch, 0).single().ok_or("invalid epoch")?;
    Ok(dt.format("%Y-%m-%d").to_string())
}

// ============================================================================
// Bank of Canada (CAD pairs)
// ============================================================================

pub struct BankOfCanadaProvider;

#[derive(Deserialize)]
struct BankOfCanadaResponse {
    observations: Vec<serde_json::Value>,
}

#[async_trait]
impl FxRateProvider for BankOfCanadaProvider {
    fn name(&self) -> &str {
        "Bank of Canada"
    }

    async fn fetch_daily_rates(&self, req: ProviderRequest) -> Result<Vec<FxRatePoint>, String> {
        let mut out = Vec::new();

        for quote in req.symbols.iter() {
            if req.base != "CAD" && quote != "CAD" {
                return Err("Bank of Canada supports only pairs with CAD".to_string());
            }
            if req.base == "CAD" && quote == "CAD" {
                continue;
            }

            let series = if req.base == "CAD" {
                format!("FXCAD{}", quote)
            } else {
                format!("FX{}CAD", req.base)
            };
            let url = format!(
                "https://www.bankofcanada.ca/valet/observations/{}?start_date={}&end_date={}",
                series, req.start_date, req.end_date
            );
            let resp = http_get(&url).await?;
            if !resp.status().is_success() {
                return Err(format!("Bank of Canada HTTP {}", resp.status()));
            }

            let payload: BankOfCanadaResponse = resp.json().await.map_err(|e| e.to_string())?;
            for obs in payload.observations {
                let date = obs.get("d").and_then(|v| v.as_str()).unwrap_or("").to_string();
                let val = obs
                    .get(&series)
                    .and_then(|v| v.get("v"))
                    .and_then(|v| v.as_str())
                    .unwrap_or("");
                let rate = val.parse::<f64>().unwrap_or(0.0);
                if date.is_empty() || rate <= 0.0 {
                    continue;
                }
                let (base, quote_out) = if req.base == "CAD" {
                    ("CAD".to_string(), quote.to_string())
                } else {
                    (req.base.clone(), "CAD".to_string())
                };

                out.push(FxRatePoint {
                    date,
                    base,
                    quote: quote_out,
                    rate,
                    source_name: self.name().to_string(),
                    source_url: Some(url.clone()),
                    is_derived: false,
                    derived_via: None,
                });
            }
        }

        Ok(out)
    }
}

// ============================================================================
// Federal Reserve (FRED series; requires API key)
// ============================================================================

pub struct FederalReserveProvider {
    pub api_key: Option<String>,
}

#[derive(Deserialize)]
struct FredResponse {
    observations: Vec<FredObservation>,
}

#[derive(Deserialize)]
struct FredObservation {
    date: String,
    value: String,
}

#[async_trait]
impl FxRateProvider for FederalReserveProvider {
    fn name(&self) -> &str {
        "Federal Reserve (FRED)"
    }

    async fn fetch_daily_rates(&self, req: ProviderRequest) -> Result<Vec<FxRatePoint>, String> {
        let api_key = self
            .api_key
            .clone()
            .or_else(|| std::env::var("FRED_API_KEY").ok())
            .ok_or_else(|| "FRED_API_KEY not set".to_string())?;

        if req.base != "USD" {
            return Err("FRED adapter supports USD base only".to_string());
        }

        let mut out = Vec::new();
        for quote in req.symbols.iter() {
            let series_id = fred_series_for_quote(quote)
                .ok_or_else(|| format!("No FRED series mapping for {}", quote))?;
            let url = format!(
                "https://api.stlouisfed.org/fred/series/observations?series_id={}&observation_start={}&observation_end={}&api_key={}&file_type=json",
                series_id, req.start_date, req.end_date, api_key
            );
            let resp = http_get(&url).await?;
            if !resp.status().is_success() {
                return Err(format!("FRED HTTP {}", resp.status()));
            }
            let payload: FredResponse = resp.json().await.map_err(|e| e.to_string())?;
            for obs in payload.observations {
                if obs.value == "." {
                    continue;
                }
                let mut rate = obs.value.parse::<f64>().unwrap_or(0.0);
                if rate <= 0.0 {
                    continue;
                }
                // FRED provides some series as USD per foreign currency.
                // For USD base → quote, invert when needed.
                if fred_series_is_usd_per_foreign(series_id) {
                    rate = 1.0 / rate;
                }
                out.push(FxRatePoint {
                    date: obs.date,
                    base: req.base.clone(),
                    quote: quote.to_string(),
                    rate,
                    source_name: self.name().to_string(),
                    source_url: Some(url.clone()),
                    is_derived: false,
                    derived_via: None,
                });
            }
        }

        Ok(out)
    }
}

fn fred_series_for_quote(quote: &str) -> Option<&'static str> {
    match quote {
        "EUR" => Some("DEXUSEU"),
        "GBP" => Some("DEXUSUK"),
        "JPY" => Some("DEXJPUS"),
        "CHF" => Some("DEXSZUS"),
        "CAD" => Some("DEXCAUS"),
        "AUD" => Some("DEXUSAL"),
        "CNY" => Some("DEXCHUS"),
        "SEK" => Some("DEXSDUS"),
        "NOK" => Some("DEXNOUS"),
        "DKK" => Some("DEXDNUS"),
        _ => None,
    }
}

fn fred_series_is_usd_per_foreign(series_id: &str) -> bool {
    matches!(series_id, "DEXUSEU" | "DEXUSUK")
}

// ============================================================================
// Cross-rate derivation
// ============================================================================

#[allow(dead_code)]
pub fn derive_cross_rates(direct: &[FxRatePoint], base: &str, via: &str) -> Vec<FxRatePoint> {
    let mut base_to_via_by_date: HashMap<String, f64> = HashMap::new();
    let mut via_to_quote_by_date: HashMap<String, Vec<(String, f64)>> = HashMap::new();

    for r in direct.iter() {
        if r.base == base && r.quote == via {
            base_to_via_by_date.insert(r.date.clone(), r.rate);
        }
        if r.base == via {
            via_to_quote_by_date
                .entry(r.date.clone())
                .or_default()
                .push((r.quote.clone(), r.rate));
        }
    }

    let mut derived = Vec::new();
    for (date, via_rate) in base_to_via_by_date {
        if let Some(quotes) = via_to_quote_by_date.get(&date) {
            for (quote, rate) in quotes.iter() {
                if quote == via {
                    continue;
                }
                derived.push(FxRatePoint {
                    date: date.clone(),
                    base: base.to_string(),
                    quote: quote.clone(),
                    rate: via_rate * rate,
                    source_name: format!("derived:{}", via),
                    source_url: None,
                    is_derived: true,
                    derived_via: Some(via.to_string()),
                });
            }
        }
    }

    derived
}

#[allow(dead_code)]
pub fn default_providers() -> Vec<Box<dyn FxRateProvider>> {
    vec![
        Box::new(FrankfurterProvider::default()),
        Box::new(FederalReserveProvider { api_key: None }),
        Box::new(YahooProvider),
        Box::new(ExchangeRateHostProvider),
        Box::new(BankOfCanadaProvider),
    ]
}

#[allow(dead_code)]
pub fn provider_by_name(name: &str) -> Option<Box<dyn FxRateProvider>> {
    match name {
        "ECB" | "ECB/Frankfurter" => Some(Box::new(FrankfurterProvider::default())),
        "Federal Reserve" | "Federal Reserve (FRED)" => {
            Some(Box::new(FederalReserveProvider { api_key: None }))
        }
        "Yahoo" | "Yahoo Finance" => Some(Box::new(YahooProvider)),
        "ExchangeRate.host" => Some(Box::new(ExchangeRateHostProvider)),
        "Bank of Canada" => Some(Box::new(BankOfCanadaProvider)),
        _ => None,
    }
}

#[allow(dead_code)]
pub fn list_provider_names() -> Vec<&'static str> {
    vec![
        "ECB/Frankfurter",
        "Federal Reserve (FRED)",
        "Yahoo Finance",
        "ExchangeRate.host",
        "Bank of Canada",
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_derive_cross_rates() {
        let direct = vec![
            FxRatePoint {
                date: "2025-01-01".to_string(),
                base: "USD".to_string(),
                quote: "EUR".to_string(),
                rate: 0.9,
                source_name: "test".to_string(),
                source_url: None,
                is_derived: false,
                derived_via: None,
            },
            FxRatePoint {
                date: "2025-01-01".to_string(),
                base: "EUR".to_string(),
                quote: "JPY".to_string(),
                rate: 160.0,
                source_name: "test".to_string(),
                source_url: None,
                is_derived: false,
                derived_via: None,
            },
        ];

        let derived = derive_cross_rates(&direct, "USD", "EUR");
        let usd_jpy = derived.iter().find(|r| r.quote == "JPY").expect("derived rate");
        assert!((usd_jpy.rate - 144.0).abs() < 0.0001);
        assert!(usd_jpy.is_derived);
        assert_eq!(usd_jpy.derived_via.as_deref(), Some("EUR"));
    }
}