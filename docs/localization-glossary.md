# Localization Glossary

The terminology decisions behind the translated message catalogs. Translating
this app term-by-term produces confident nonsense: it is a double-entry ledger
wearing a personal-finance coat, so half the vocabulary is accounting
vocabulary with an established word in each language, and the other half is
consumer banking vocabulary with a *different* established word.

**Review this file, not the 1,163 strings.** Every catalog follows these
choices; if a term is wrong here it is wrong in a hundred places, and if it is
right here the rest is mechanical.

Target locales, decided 2026-08-19: Spanish, French, Dutch, German, Russian.

## Sources the choices are anchored to

Where a term already exists in the software this app replaces or competes with,
that term wins over a more literal translation:

- **GnuCash** — the only widely translated open-source double-entry ledger; its
  locale teams have argued about exactly these words for two decades.
- **MS Money and Quicken** localized editions — the vocabulary the migrating
  user already has in their head.
- Everyday **banking-statement** language per market, for the consumer half
  (statement, balance, direct debit).

## Core ledger terms

| English | Spanish | French | Dutch | German | Russian |
|---|---|---|---|---|---|
| Account | Cuenta | Compte | Rekening | Konto | Счёт |
| Transaction | Transacción | Transaction | Transactie | Buchung | Транзакция |
| Posting | Apunte | Écriture | Boekingsregel | Buchungszeile | Проводка |
| Ledger | Libro mayor | Grand livre | Grootboek | Hauptbuch | Главная книга |
| Balance | Saldo | Solde | Saldo | Saldo | Остаток |
| Category | Categoría | Catégorie | Categorie | Kategorie | Категория |
| Payee | Beneficiario | Bénéficiaire | Begunstigde | Empfänger | Получатель |
| Split (UI) | Desglose | Ventilation | Splitsing | Aufteilung | Разбивка |
| Transfer | Transferencia | Virement | Overboeking | Umbuchung | Перевод |

**Transaction vs posting is the pair that matters most.** A transaction holds
postings; both need distinct words or the reconciliation and split screens stop
making sense. German is the sharpest case: *Buchung* is the accounting-correct
word for the whole transaction, so a posting has to be *Buchungszeile* rather
than reusing *Buchung* for both. Spanish *apunte* and French *écriture* are the
native accounting terms for the line, not inventions.

## Report terms

| English | Spanish | French | Dutch | German | Russian |
|---|---|---|---|---|---|
| Net worth | Patrimonio neto | Patrimoine net | Nettovermogen | Nettovermögen | Чистые активы |
| Cashflow | Flujo de caja | Flux de trésorerie | Kasstroom | Cashflow | Денежный поток |
| Spending | Gastos | Dépenses | Uitgaven | Ausgaben | Расходы |
| Income | Ingresos | Revenus | Inkomsten | Einnahmen | Доходы |
| Reconcile | Conciliar | Rapprocher | Afstemmen | Abgleichen | Сверить |
| Reconciliation | Conciliación | Rapprochement | Afstemming | Kontenabgleich | Сверка |
| Statement | Extracto | Relevé | Afschrift | Kontoauszug | Выписка |

German keeps **Cashflow** untranslated because German finance writing does;
*Geldfluss* exists but reads like a textbook rather than a product.

## Two deliberate departures from the literal

**Commodity → "Instrument".** In this ledger a *commodity* is "a currency or a
security" — the unit a quantity is denominated in. The literal translation in
all five languages (*mercancía, marchandise, grondstof, Rohstoff, товар*) means
physical goods and would be simply wrong. *Instrument* / *Инструмент* covers a
currency and a share alike and is finance-native in every target language.
Calling it *Divisa/Devise/Valuta/Währung/Валюта* would be equally wrong the
moment a share appears.

**Cleared vs reconciled** are different states and must not collapse into one
word. *Cleared* means "this posting appears on the statement"; *reconciled*
means "the whole account is locked to that statement". These are the entries
most worth a native check:

| English | Spanish | French | Dutch | German | Russian |
|---|---|---|---|---|---|
| Cleared | Punteado | Pointé | Bevestigd | Bestätigt | Подтверждено |
| Reconciled | Conciliado | Rapproché | Afgestemd | Abgeglichen | Сверено |

French *pointé* is certain — pointing off a statement is the standard phrase.
Spanish *punteado* is the traditional accounting term but is less common in
consumer products than *conciliado*, which is already taken by "reconciled";
**this is the first thing to check with a native speaker.**

## Register

The English copy is calm and declarative: full sentences for explanation, terse
labels for controls, no exclamation marks, no cheerfulness at the user. The
translations keep that.

- **Formality:** the polite-but-not-stiff register. German *Sie*, Russian «вы»,
  French *vous*, Spanish *usted* implied — but the copy is written so that
  direct address is rare, which sidesteps most of it.
- **Imperatives on buttons**, as in English: *Guardar*, *Enregistrer*,
  *Opslaan*, *Speichern*, *Сохранить*.
- **Numbers, dates, and money are never translated** — they are formatted by
  `Intl` from the active locale, so a catalog must never bake in a separator or
  a currency symbol.

## Mechanics that constrain wording

- **Russian brings a three-form plural rule** (1 / 2–4 / 5+) and Cyrillic. Any
  message that counts things needs checking against it rather than assuming the
  two-form Latin behaviour; the current catalogs avoid bare counted nouns for
  this reason.
- **German and Dutch compound**, so labels run 30–50% longer than English.
  Table headers were chosen short for this reason (*Ein/Aus*, *In/Uit*).
- Built-in database labels stay stable codes; only the display layer is
  translated. Never translate a code.
