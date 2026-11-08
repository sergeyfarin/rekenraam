package app

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

// #379 measured the former global entitlement admission. #388 removes that
// policy; these same history shapes now measure accepted corrections, unrelated
// writes and rolled-back previews. The diagnostic is measured separately.
//
//	./scripts/measure-payment-in-lieu-density.sh
//
// Fixture construction is outside every measured operation and reported separately.

func paymentDensityEnabled(tb testing.TB) {
	tb.Helper()
	if os.Getenv("REKENRAAM_PIL_DENSITY") != "1" {
		tb.Skip("payment-in-lieu density measurement runs only with REKENRAAM_PIL_DENSITY=1 (scripts/measure-payment-in-lieu-density.sh)")
	}
}

// serial: opt-in wall/allocation measurement must run without other tests.
func TestPaymentInLieuAttributionCostByDensity(t *testing.T) {
	paymentDensityEnabled(t)
	ctx := context.Background()
	for _, shape := range []string{paymentShapeUnrelated, paymentShapeConcentrated} {
		for _, density := range paymentDensityInts(t, "REKENRAAM_PIL_DENSITIES", []int{0, 10, 100}) {
			t.Run(fmt.Sprintf("%s/%d", shape, density), func(t *testing.T) {
				book := buildPaymentDensityBook(t, shape, density)
				repo := db.NewSelfCheckRepository(book.f.database, book.f.database)
				for sample := range 3 {
					probe := startResourceProbe()
					snapshot, err := repo.Snapshot(ctx)
					require.NoError(t, err)
					records, err := repo.PaymentInLieuAttributions(ctx, snapshot, BookID)
					require.NoError(t, err)
					require.NoError(t, snapshot.Rollback())
					usage := probe.finish()
					require.Len(t, records, density)
					for _, record := range records {
						entitled, exists := book.entitled[record.TransactionID]
						require.True(t, exists)
						require.False(t, record.Unavailable)
						require.Equal(t, !entitled, len(record.WarningReasons) > 0)
					}
					t.Logf("attribution shape=%s density=%d sample=%d wall=%s allocated=%d peak_heap=%d fixture_build=%s", shape, density, sample, usage.Wall, usage.Allocated, usage.PeakHeap, book.build)
				}
			})
		}
	}
}

func paymentDensityInts(tb testing.TB, name string, fallback []int) []int {
	tb.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	var values []int
	for field := range strings.SplitSeq(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(field))
		require.NoError(tb, err, "%s", name)
		values = append(values, value)
	}
	return values
}

// Two history shapes over one holding account. "unrelated": every payment
// is owed on its own instrument's single short (one short each).
// "concentrated": every background payment is owed on one instrument whose
// short history grows by one sale per payment, exercising the same historically expensive shape.
const (
	paymentShapeUnrelated    = "unrelated"
	paymentShapeConcentrated = "concentrated"
)

type paymentDensityBook struct {
	f        *investmentsTestFixture
	expense  int64
	usd      int64
	target   int64 // the protected short's instrument
	targetTx int64 // its current short sale transaction
	// targetPayment is the payment owed on the target short (zero at density 0).
	targetPayment int64
	// entitled records, by construction, which effective payments hold their
	// short entitlement; the unsupported control is false.
	entitled  map[int64]bool
	reversed  int64 // superseded control payment (not effective)
	effective int
	build     time.Duration
	buildTail []time.Duration // wall of the last few background payment writes
	nextCode  int
}

func (b *paymentDensityBook) security(t *testing.T) int64 {
	t.Helper()
	b.nextCode++
	return seedTestSecurityCommodity(t, b.f, fmt.Sprintf("D%05d", b.nextCode))
}

func (b *paymentDensityBook) short(t *testing.T, commodity int64, date string, quantity, cash, cashCommodity int64) InvestmentTradeResult {
	t.Helper()
	input := shortInput(b.f, date, quantity, cash)
	input.CommodityID, input.CashCommodityID = commodity, cashCommodity
	result, err := b.f.investmentService.ShortSale(context.Background(), input)
	require.NoError(t, err)
	return result
}

func (b *paymentDensityBook) payment(t *testing.T, commodity int64, exDate, date string, entitled bool) int64 {
	t.Helper()
	input := paymentInLieuInput(b.f, b.expense, 500)
	input.CommodityID, input.ExDate, input.TransactionDate = NullableValue(commodity), NullableValue(exDate), date
	if !entitled {
		input.HoldingAccountID = NullableValue(seedTestAccountWithClass(t, b.f.database, "active", true, "asset", "security_holding"))
	}
	started := time.Now()
	payment, err := b.f.investmentService.PaymentInLieu(context.Background(), input)
	require.NoError(t, err)
	b.buildTail = append(b.buildTail, time.Since(started))
	if len(b.buildTail) > 5 {
		b.buildTail = b.buildTail[1:]
	}
	b.entitled[payment.ID] = entitled
	b.effective++
	return payment.ID
}

func paymentDay(start string, offset int) string {
	day, err := time.Parse(time.DateOnly, start)
	if err != nil {
		panic(err)
	}
	return day.AddDate(0, 0, offset).Format(time.DateOnly)
}

// buildPaymentDensityBook records `payments` effective payments in lieu:
// the target's, one unsupported control, and the shape's background; plus one
// reversed (superseded) control. Density 0 is the payment-free control.
func buildPaymentDensityBook(t *testing.T, shape string, payments int) *paymentDensityBook {
	t.Helper()
	ctx := context.Background()
	started := time.Now()
	f := newInvestmentsTestFixture(t)
	b := &paymentDensityBook{f: f, entitled: map[int64]bool{}}
	b.expense = seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	b.usd = seedTestCurrencyCommodity(t, f.database, "USD")

	background := max(payments-2, 0) // the target and the unsupported control are counted in `payments`
	switch shape {
	case paymentShapeUnrelated:
		for index := range background {
			commodity := b.security(t)
			if index%10 == 1 {
				// Two cost currencies: the EUR short is covered before the
				// ex-date, so the check iterates past it to the open USD short.
				b.short(t, commodity, "2026-02-02", 10, 100000, f.eurCommodityID)
				cover := coverInput(f, "2026-02-16", 10, 90000, "fifo")
				cover.CommodityID = commodity
				_, err := f.investmentService.ShortCover(ctx, cover)
				require.NoError(t, err)
				b.short(t, commodity, "2026-02-17", 10, 110000, b.usd)
			} else {
				cashCommodity := f.eurCommodityID
				if index%2 == 1 {
					cashCommodity = b.usd
				}
				b.short(t, commodity, "2026-02-02", 10, 100000, cashCommodity)
			}
			b.payment(t, commodity, "2026-03-02", "2026-03-04", true)
		}
	case paymentShapeConcentrated:
		// Sales spread over 2026-01-02..2026-08-30 (the fixture accounts open
		// 2026-01-01); each payment's ex-date is the day after a sale, so its
		// entitlement replays every sale before it.
		commodity := f.stockCommodityID
		for index := range background {
			cashCommodity := f.eurCommodityID
			if index%2 == 1 {
				cashCommodity = b.usd
			}
			day := index * 240 / background
			b.short(t, commodity, paymentDay("2026-01-02", day), 1, 10000, cashCommodity)
			b.payment(t, commodity, paymentDay("2026-01-02", day+1), paymentDay("2026-01-02", day+1), true)
		}
	default:
		t.Fatalf("unknown shape %q", shape)
	}

	b.target = b.security(t)
	b.targetTx = b.short(t, b.target, "2026-04-01", 10, 100000, f.eurCommodityID).Transaction.ID
	if payments > 0 {
		b.targetPayment = b.payment(t, b.target, "2026-04-15", "2026-04-17", true)
	}
	if payments > 1 {
		// Damaged control: its facts point at the cash account, which was
		// never short, so it is already unentitled (a self-check finding).
		commodity := b.security(t)
		b.short(t, commodity, "2026-04-01", 10, 100000, f.eurCommodityID)
		b.payment(t, commodity, "2026-04-15", "2026-04-17", false)
		// Superseded control: owed, then reversed; no longer effective.
		commodity = b.security(t)
		b.short(t, commodity, "2026-04-01", 10, 100000, f.eurCommodityID)
		b.reversed = b.payment(t, commodity, "2026-04-15", "2026-04-17", true)
		delete(b.entitled, b.reversed)
		b.effective--
		_, err := f.investmentService.ReversePaymentInLieu(ctx, ReverseInvestmentPaymentInLieuInput{
			OwnerUserID: f.ownerUserID, TransactionID: b.reversed, Reason: "superseded control"})
		require.NoError(t, err)
	}
	require.Equal(t, payments, b.effective)
	b.build = time.Since(started)
	return b
}

// requireEntitlementsAsConstructed compares the durable effective payments,
// and the self-check's orphan finding, with what the build recorded.
func (b *paymentDensityBook) requireEntitlementsAsConstructed(t *testing.T) {
	t.Helper()
	rows, err := b.f.database.Query(`SELECT v.transaction_id FROM effective_investment_operations o
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.role = 'primary'
		JOIN transaction_versions v ON v.id = link.transaction_version_id
		WHERE o.operation_kind = 'payment_in_lieu' ORDER BY v.transaction_id`)
	require.NoError(t, err)
	var effective []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		effective = append(effective, id)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	var expected []int64
	unentitled := 0
	for id, entitled := range b.entitled {
		expected = append(expected, id)
		if !entitled {
			unentitled++
		}
	}
	slices.Sort(expected)
	require.Equal(t, expected, effective, "effective payments are exactly the constructed ones")

	result := resultFor(t, mustRunInvestmentSelfCheck(t, b.f), CheckInvestmentAttribution)
	if unentitled == 0 {
		require.Equal(t, SelfCheckPassed, result.Status, result.Summary)
	} else {
		require.Equal(t, SelfCheckWarning, result.Status)
		require.EqualValues(t, unentitled, result.WarningCount)
	}
}

type paymentDensitySample struct {
	usage  resourceUsage
	queued time.Duration
}

// measurePaymentDensityOperation runs op once under the resource probe.
// Trivial reads queue on the single writer connection back to back while op
// runs; the longest wait is how long op held that connection in one stretch
// (its write transaction, since the prepare phase releases it between reads).
func measurePaymentDensityOperation(t *testing.T, b *paymentDensityBook, op func() error) (paymentDensitySample, error) {
	t.Helper()
	probe := startResourceProbe()
	stop := make(chan struct{})
	queued := make(chan time.Duration, 1)
	go func() {
		var longest time.Duration
		for {
			select {
			case <-stop:
				queued <- longest
				return
			default:
			}
			started := time.Now()
			var one int
			_ = b.f.database.QueryRow(`SELECT 1`).Scan(&one)
			longest = max(longest, time.Since(started))
			time.Sleep(200 * time.Microsecond)
		}
	}()
	err := op()
	usage := probe.finish()
	close(stop)
	return paymentDensitySample{usage: usage, queued: <-queued}, err
}

func logPaymentDensitySamples(t *testing.T, shape string, payments int, op string, samples []paymentDensitySample) {
	t.Helper()
	walls := make([]time.Duration, 0, len(samples))
	var allocated uint64
	var queued time.Duration
	for _, sample := range samples {
		walls = append(walls, sample.usage.Wall)
		allocated += sample.usage.Allocated
		queued = max(queued, sample.queued)
	}
	slices.Sort(walls)
	t.Logf("RESULT shape=%s payments=%d op=%s runs=%d wall_min=%s wall_median=%s wall_max=%s alloc_mean_KiB=%d writer_wait_max=%s",
		shape, payments, op, len(samples), walls[0].Round(10*time.Microsecond), walls[len(walls)/2].Round(10*time.Microsecond),
		walls[len(walls)-1].Round(10*time.Microsecond), allocated/uint64(len(samples))>>10, queued.Round(10*time.Microsecond))
}

// serial: measures wall time and process-wide allocation; parallel tests
// would contend for the same cores and heap counters.
func TestPaymentInLieuAdmissionCostByDensity(t *testing.T) {
	paymentDensityEnabled(t)
	ctx := context.Background()
	shapes := []string{paymentShapeUnrelated, paymentShapeConcentrated}
	if raw := os.Getenv("REKENRAAM_PIL_SHAPES"); raw != "" {
		shapes = strings.Split(raw, ",")
	}
	densities := paymentDensityInts(t, "REKENRAAM_PIL_DENSITIES", []int{0, 10, 100, 300})
	repeats := paymentDensityInts(t, "REKENRAAM_PIL_REPEATS", []int{5})[0]

	for _, shape := range shapes {
		for _, payments := range densities {
			t.Run(fmt.Sprintf("%s/%d", shape, payments), func(t *testing.T) {
				b := buildPaymentDensityBook(t, shape, payments)
				t.Logf("BUILD shape=%s payments=%d wall=%s last_payment_writes=%v", shape, payments,
					b.build.Round(time.Millisecond), b.buildTail)
				b.requireEntitlementsAsConstructed(t)
				f := b.f

				// Successful short correction: new proceeds, same date. The
				// target's payment stays effective with supported attribution.
				var previews, commits []paymentDensitySample
				for repeat := range repeats {
					replacement := shortInput(f, "2026-04-01", 10, int64(100000+100*(repeat+1)))
					replacement.CommodityID = b.target
					input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: b.targetTx,
						Reason: "proceeds corrected", Replacement: replacement}
					before := buyReplacementPreviewSnapshot(t, b.f.database)
					sample, err := measurePaymentDensityOperation(t, b, func() error {
						_, err := f.investmentService.ReplaceShortSaleReconciliationImpact(ctx, input)
						return err
					})
					require.NoError(t, err)
					require.Equal(t, before, buyReplacementPreviewSnapshot(t, b.f.database), "preview wrote nothing")
					previews = append(previews, sample)
					var result ReplaceInvestmentBuyResult
					sample, err = measurePaymentDensityOperation(t, b, func() error {
						var err error
						result, err = f.investmentService.ReplaceShortSale(ctx, input)
						return err
					})
					require.NoError(t, err)
					commits = append(commits, sample)
					b.targetTx = result.Replacement.Transaction.ID
				}
				logPaymentDensitySamples(t, shape, payments, "short_replacement_preview", previews)
				logPaymentDensitySamples(t, shape, payments, "short_replacement", commits)

				// Unrelated command: a short sale of an instrument nobody owes
				// on, previewed then committed.
				previews, commits = nil, nil
				for range repeats {
					input := shortInput(f, "2026-06-01", 5, 50000)
					input.CommodityID = b.security(t)
					before := buyReplacementPreviewSnapshot(t, b.f.database)
					sample, err := measurePaymentDensityOperation(t, b, func() error {
						_, err := f.investmentService.shortSaleReconciliationImpact(ctx, input)
						return err
					})
					require.NoError(t, err)
					require.Equal(t, before, buyReplacementPreviewSnapshot(t, b.f.database), "preview wrote nothing")
					previews = append(previews, sample)
					sample, err = measurePaymentDensityOperation(t, b, func() error {
						_, err := f.investmentService.ShortSale(ctx, input)
						return err
					})
					require.NoError(t, err)
					commits = append(commits, sample)
				}
				logPaymentDensitySamples(t, shape, payments, "unrelated_short_sale_preview", previews)
				logPaymentDensitySamples(t, shape, payments, "unrelated_short_sale", commits)

				// Reversal is now admissible with the payment left effective.
				// Reopen outside timing between samples to keep the financial
				// position and attributed payment fixed for each measurement.
				previews, commits = nil, nil
				for range repeats {
					input := ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: b.targetTx, Reason: "not shorted"}
					before := buyReplacementPreviewSnapshot(t, f.database)
					sample, err := measurePaymentDensityOperation(t, b, func() error {
						_, err := f.investmentService.ReverseShortSaleReconciliationImpact(ctx, input)
						return err
					})
					require.NoError(t, err)
					require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
					previews = append(previews, sample)
					sample, err = measurePaymentDensityOperation(t, b, func() error { _, err := f.investmentService.ReverseShortSale(ctx, input); return err })
					require.NoError(t, err)
					commits = append(commits, sample)
					if b.targetPayment != 0 {
						requirePaymentAttributionWarning(t, f, b.targetPayment)
					}
					b.targetTx = b.short(t, b.target, "2026-04-01", 10, int64(100000+100*repeats), f.eurCommodityID).Transaction.ID
				}
				logPaymentDensitySamples(t, shape, payments, "short_reversal_preview", previews)
				logPaymentDensitySamples(t, shape, payments, "short_reversal", commits)

				b.requireEntitlementsAsConstructed(t)
			})
		}
	}
}
