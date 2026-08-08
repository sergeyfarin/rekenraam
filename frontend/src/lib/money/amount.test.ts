import { describe, it, expect } from 'vitest';
import {
	commodityImbalance,
	sumByCommodity,
	formatLedgerAmount,
	inflowPositiveAmount,
	negateCoefficient,
	parseDecimalAmount,
	MAX_COEFFICIENT_DIGITS
} from './amount';

describe('parseDecimalAmount', () => {
	const accepted: [string, string, number][] = [
		// input, coefficient, scale
		['1234', '1234', 0],
		['1234.56', '123456', 2],
		['100.00', '10000', 2],
		['0.05', '5', 2],
		['-42.10', '-4210', 2],
		['007.50', '750', 2],
		['  12.00  ', '1200', 2],
		['1,234.56', '123456', 2],
		['1,234,567', '1234567', 0],
		['0.00000001', '1', 8]
	];

	it.each(accepted)('parses %s', (input, value, scale) => {
		expect(parseDecimalAmount(input)).toEqual({ value, scale });
	});

	const rejected = [
		'',
		'   ',
		'abc',
		'1.2.3',
		'$5',
		'1 234',
		'--5',
		'.',
		'-',
		'1.',
		'+25',
		'1e3'
	];

	it.each(rejected)('rejects %j', (input) => {
		expect(parseDecimalAmount(input)).toBeNull();
	});

	describe('decimal-comma input', () => {
		// The `en` locale reads "," as a thousands separator. Stripping every
		// comma would silently read "1,50" as 150 — a 100x error. Malformed
		// grouping is rejected so the form can complain instead of guessing.
		it.each(['1,50', '12,3', '1,2345', '1,23,456', ',50', '1,'])(
			'rejects ungrouped comma input %j rather than guessing',
			(input) => {
				expect(parseDecimalAmount(input)).toBeNull();
			}
		);

		it('still accepts well-formed thousands grouping', () => {
			expect(parseDecimalAmount('1,500')).toEqual({ value: '1500', scale: 0 });
		});
	});

	describe('negative and zero amounts', () => {
		it.each([
			['0', '0', 0],
			['0.00', '0', 2],
			['-0', '0', 0],
			['-0.00', '0', 2],
			['-0.01', '-1', 2]
		])('normalises %s to an unsigned zero magnitude', (input, value, scale) => {
			expect(parseDecimalAmount(input)).toEqual({ value, scale });
		});
	});

	describe('beyond Number.MAX_SAFE_INTEGER', () => {
		it('keeps every digit of a value past 2^53', () => {
			expect(parseDecimalAmount('90071992547409.93')).toEqual({
				value: '9007199254740993',
				scale: 2
			});
		});

		it('does not lose precision on a 30-digit coefficient', () => {
			const input = '1234567890123456789012345678.90';
			expect(parseDecimalAmount(input)).toEqual({
				value: '123456789012345678901234567890',
				scale: 2
			});
		});

		it('rejects a coefficient the backend would refuse', () => {
			const tooLong = '9'.repeat(MAX_COEFFICIENT_DIGITS + 1);
			expect(parseDecimalAmount(tooLong)).toBeNull();
			expect(parseDecimalAmount('9'.repeat(MAX_COEFFICIENT_DIGITS))).not.toBeNull();
		});
	});

	describe('scale ceilings', () => {
		it('rejects a scale past the crypto ceiling', () => {
			expect(parseDecimalAmount(`0.${'1'.repeat(25)}`)).toBeNull();
			expect(parseDecimalAmount(`0.${'1'.repeat(24)}`)).not.toBeNull();
		});

		it("honours a commodity's tighter max scale", () => {
			expect(parseDecimalAmount('1.123456789', { maxScale: 2 })).toBeNull();
			expect(parseDecimalAmount('1.12', { maxScale: 2 })).toEqual({ value: '112', scale: 2 });
		});
	});
});

describe('formatLedgerAmount', () => {
	it.each([
		['1234', 0, '1234'],
		['123456', 2, '1234.56'],
		['5', 2, '0.05'],
		['-4210', 2, '-42.10'],
		['-5', 2, '-0.05'],
		['0', 2, '0.00'],
		['1', 8, '0.00000001'],
		['9007199254740993', 2, '90071992547409.93']
	])('renders %s at scale %i as %s', (value, scale, expected) => {
		expect(formatLedgerAmount(value, scale)).toBe(expected);
	});

	it('round-trips through the parser', () => {
		for (const input of ['1234.56', '-42.10', '0.05', '0.00', '90071992547409.93']) {
			const parsed = parseDecimalAmount(input)!;
			const rendered = formatLedgerAmount(parsed.value, parsed.scale);
			expect(parseDecimalAmount(rendered)).toEqual(parsed);
		}
	});
});

describe('negateCoefficient', () => {
	it.each([
		['100', '-100'],
		['-100', '100'],
		['0', '0'],
		['0.00', '0.00'],
		['-0.00', '0.00'],
		['12.50', '-12.50'],
		['', '0']
	])('negates %j to %j', (input, expected) => {
		expect(negateCoefficient(input)).toBe(expected);
	});
});

describe('inflowPositiveAmount', () => {
	it('leaves an asset posting debit-positive', () => {
		expect(
			inflowPositiveAmount({ quantity_value: '2500', quantity_scale: 2, account_class: 'asset' })
		).toBe('25.00');
	});

	it('leaves an expense posting debit-positive', () => {
		expect(
			inflowPositiveAmount({ quantity_value: '-2500', quantity_scale: 2, account_class: 'expense' })
		).toBe('-25.00');
	});

	it.each(['liability', 'income', 'equity'])('flips the sign for a %s posting', (accountClass) => {
		expect(
			inflowPositiveAmount({
				quantity_value: '-2500',
				quantity_scale: 2,
				account_class: accountClass
			})
		).toBe('25.00');
	});

	it('does not sign a zero liability posting', () => {
		expect(
			inflowPositiveAmount({ quantity_value: '0', quantity_scale: 2, account_class: 'liability' })
		).toBe('0.00');
	});
});

describe('commodityImbalance', () => {
	it('reports no imbalance for legs that sum to zero', () => {
		expect(
			commodityImbalance([
				{ commodityID: '1', amountStr: '100.00' },
				{ commodityID: '1', amountStr: '-100.00' }
			])
		).toBeNull();
	});

	it('reports the residual when legs do not sum to zero', () => {
		expect(
			commodityImbalance([
				{ commodityID: '1', amountStr: '100.00' },
				{ commodityID: '1', amountStr: '-60.00' }
			])
		).toEqual({ commodityID: '1', amount: '40.00' });
	});

	describe('scale mismatch', () => {
		it('aligns legs typed at different scales before summing', () => {
			expect(
				commodityImbalance([
					{ commodityID: '1', amountStr: '1.5' },
					{ commodityID: '1', amountStr: '-1.50' }
				])
			).toBeNull();
		});

		it('reports the residual at the widest scale seen', () => {
			expect(
				commodityImbalance([
					{ commodityID: '1', amountStr: '10' },
					{ commodityID: '1', amountStr: '-9.999' }
				])
			).toEqual({ commodityID: '1', amount: '0.001' });
		});

		it('aligns when the wider scale arrives first', () => {
			expect(
				commodityImbalance([
					{ commodityID: '1', amountStr: '-1.50' },
					{ commodityID: '1', amountStr: '1.5' }
				])
			).toBeNull();
		});

		it('handles a scale widening twice across three legs', () => {
			expect(
				commodityImbalance([
					{ commodityID: '1', amountStr: '1' },
					{ commodityID: '1', amountStr: '-0.5' },
					{ commodityID: '1', amountStr: '-0.50' }
				])
			).toBeNull();
		});
	});

	describe('multiple commodities', () => {
		it('balances each commodity independently', () => {
			expect(
				commodityImbalance([
					{ commodityID: '1', amountStr: '100.00' },
					{ commodityID: '1', amountStr: '-100.00' },
					{ commodityID: '2', amountStr: '5.00000000' },
					{ commodityID: '2', amountStr: '-5.0' }
				])
			).toBeNull();
		});

		it('never nets one commodity against another', () => {
			expect(
				commodityImbalance([
					{ commodityID: '1', amountStr: '100.00' },
					{ commodityID: '2', amountStr: '-100.00' }
				])
			).toEqual({ commodityID: '1', amount: '100.00' });
		});
	});

	it('ignores blank and incomplete legs', () => {
		expect(
			commodityImbalance([
				{ commodityID: '1', amountStr: '100.00' },
				{ commodityID: '1', amountStr: '-100.00' },
				{ commodityID: '', amountStr: '50.00' },
				{ commodityID: '1', amountStr: '   ' }
			])
		).toBeNull();
	});

	it('ignores unparseable legs, which callers must reject separately', () => {
		expect(
			commodityImbalance([
				{ commodityID: '1', amountStr: '100.00' },
				{ commodityID: '1', amountStr: '-100.00' },
				{ commodityID: '1', amountStr: '1,50' }
			])
		).toBeNull();
	});

	it('stays exact past Number.MAX_SAFE_INTEGER', () => {
		expect(
			commodityImbalance([
				{ commodityID: '1', amountStr: '90071992547409.93' },
				{ commodityID: '1', amountStr: '-90071992547409.92' }
			])
		).toEqual({ commodityID: '1', amount: '0.01' });
	});

	it('returns null for no legs at all', () => {
		expect(commodityImbalance([])).toBeNull();
	});
});

describe('sumByCommodity', () => {
  const posting = (
    commodity_id: number,
    quantity_value: string,
    quantity_scale: number,
    account_class: string
  ) => ({ commodity_id, quantity_value, quantity_scale, account_class });

  it('returns nothing for no postings', () => {
    expect(sumByCommodity([])).toEqual([]);
  });

  it('keeps an expense debit positive', () => {
    expect(sumByCommodity([posting(1, '3000', 2, 'expense')])).toEqual([
      { commodityID: 1, value: '3000', scale: 2 }
    ]);
  });

  it('flips an income credit so activity reads positive', () => {
    // Storage is debit-positive, so income arrives negative. The normal-sign
    // convention makes it read as a positive amount of income.
    expect(sumByCommodity([posting(1, '-200000', 2, 'income')])).toEqual([
      { commodityID: 1, value: '200000', scale: 2 }
    ]);
  });

  it.each([
    ['liability', '-500', '500'],
    ['equity', '-500', '500'],
    ['asset', '500', '500'],
    ['expense', '500', '500']
  ])('applies the normal-sign rule for %s', (accountClass, raw, expected) => {
    expect(sumByCommodity([posting(1, raw, 2, accountClass)])[0].value).toBe(expected);
  });

  it('aligns differing scales before adding', () => {
    // 5.00 (scale 2) + 0.1234 (scale 4) = 5.1234, not 500 + 1234.
    expect(sumByCommodity([posting(1, '500', 2, 'expense'), posting(1, '1234', 4, 'expense')])).toEqual([
      { commodityID: 1, value: '51234', scale: 4 }
    ]);
  });

  it('keeps commodities separate rather than summing across them', () => {
    const totals = sumByCommodity([posting(1, '500', 2, 'expense'), posting(2, '300', 2, 'expense')]);

    expect(totals).toEqual([
      { commodityID: 1, value: '500', scale: 2 },
      { commodityID: 2, value: '300', scale: 2 }
    ]);
  });

  it('returns commodities in first-seen order, not map order', () => {
    const totals = sumByCommodity([
      posting(9, '100', 2, 'expense'),
      posting(2, '100', 2, 'expense'),
      posting(9, '100', 2, 'expense')
    ]);

    expect(totals.map((total) => total.commodityID)).toEqual([9, 2]);
  });

  it('nets a refund against its own category', () => {
    expect(sumByCommodity([posting(1, '5000', 2, 'expense'), posting(1, '-2000', 2, 'expense')])).toEqual([
      { commodityID: 1, value: '3000', scale: 2 }
    ]);
  });

  it('stays exact past Number.MAX_SAFE_INTEGER', () => {
    const totals = sumByCommodity([
      posting(1, '9007199254740993', 2, 'expense'),
      posting(1, '1', 2, 'expense')
    ]);

    expect(totals[0].value).toBe('9007199254740994');
  });

  it('mixes classes within one commodity using each posting’s own rule', () => {
    // Both legs are positive once normal-signed — 30.00 of expense activity
    // plus 200.00 of income activity — so they add to 230.00 rather than
    // cancelling. Normal sign measures activity, not direction of cash.
    expect(sumByCommodity([posting(1, '3000', 2, 'expense'), posting(1, '-20000', 2, 'income')])).toEqual([
      { commodityID: 1, value: '23000', scale: 2 }
    ]);
  });
});
