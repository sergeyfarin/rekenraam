import { expect, test, type Page } from '@playwright/test';
import { apiJSON } from './support/api';
import { readyForLedger, ensureCurrency } from './support/ledger';
import { todayISO } from './support/dates';

test.use({ viewport: { width: 390, height: 844 } });
function ago(days:number) { const date=new Date();date.setDate(date.getDate()-days);return todayISO(date); }

async function setup(page:Page, multipleLots=false) {
  const {csrfToken,currencyID}=await readyForLedger(page);
  const suffix=`cil${Date.now()}`;
  const cash=await apiJSON<{id:number}>(page,'POST','/api/v1/accounts',csrfToken,{name:`CIL cash ${suffix}`,account_class:'asset',account_kind:'brokerage_cash',default_commodity_id:currencyID,allows_postings:true,opened_on:ago(60),effective_from:ago(60)});
  const instrument=await apiJSON<{id:number;commodity_id:number}>(page,'POST','/api/v1/investments/instruments',csrfToken,{commodity_code:suffix.toUpperCase(),instrument_type:'stock',display_name:`CIL ${suffix}`,symbol:suffix.toUpperCase(),quote_commodity_id:currencyID,trading_commodity_id:currencyID,quantity_scale:3,price_scale:2,effective_from:ago(60)});
  const holding=await apiJSON<{id:number}>(page,'POST','/api/v1/investments/holding-accounts',csrfToken,{instrument_id:instrument.id,name:`CIL holding ${suffix}`,opened_on:ago(60),effective_from:ago(60)});
  const trade=(side:'buy'|'sell',days:number,q:string,amount:string)=>apiJSON(page,'POST',`/api/v1/investments/${side}`,csrfToken,{transaction_date:ago(days),commodity_id:instrument.commodity_id,holding_account_id:holding.id,cash_account_id:cash.id,quantity_value:q,quantity_scale:0,cash_amount_value:amount,cash_amount_scale:2,cash_commodity_id:currencyID,cost_basis_method:'fifo'});
  if (multipleLots) {await trade('buy',40,'1','600');await trade('buy',39,'1','2400');}
  else await trade('buy',40,'3','3000');
  const split=await apiJSON<{transaction:{id:number}}>(page,'POST','/api/v1/investments/splits',csrfToken,{effective_on:ago(30),holding_account_id:holding.id,commodity_id:instrument.commodity_id,ratio_numerator:3,ratio_denominator:2});
  return {csrfToken,currencyID,cash,holding,instrument,name:`CIL ${suffix}`,splitID:split.transaction.id,trade};
}

async function entry(page:Page,p:Awaited<ReturnType<typeof setup>>,days=30) {
  await page.goto(`/app/transactions?transaction_id=${p.splitID}`);
  await page.getByRole('button',{name:'Record cash in lieu…',exact:true}).click();
  const form=page.getByRole('dialog',{name:'Record cash in lieu',exact:true});
  await expect(form.getByLabel('Disposal date')).toHaveValue(ago(30));
  await form.getByLabel('Fraction disposed').fill('0.5');
  await form.getByLabel('Disposal date').fill(ago(days));
  await form.getByLabel('Payment date').fill(ago(days-5));
  await form.getByLabel('Received into').selectOption(String(p.cash.id));
  await form.getByLabel('Cash proceeds').fill('8.00');
  return form;
}

test('mobile cash in lieu is entered with specific lots, corrected, reversed, and releases its split',async({page})=>{
  const p=await setup(page);
  const form=await entry(page,p);
  await form.getByLabel('Cost-basis method').selectOption('specific_lot');
  await form.getByLabel(/quantity to dispose \(available:/).fill('0.5');
  await form.getByRole('button',{name:'Preview cash in lieu',exact:true}).click();
  const plan=form.getByRole('region',{name:'Fraction allocation and realized result'});
  await expect(plan).toContainText('Basis 3.333333');
  await expect(plan).toContainText('realized result 4.666667');
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  const posted=page.waitForResponse(r=>r.url().endsWith('/api/v1/investments/cash-in-lieu') && r.request().method()==='POST');
  await form.getByRole('button',{name:'Record cash in lieu',exact:true}).click();
  const original=await(await posted).json();
  await expect(form).toBeHidden();
  await page.goto(`/app/transactions?transaction_id=${original.transaction.id}`);
  await page.getByRole('button',{name:'Correct cash in lieu',exact:true}).click();
  const correction=page.getByRole('dialog',{name:'Correct cash in lieu',exact:true});
  await expect(correction.getByLabel('Fraction disposed')).toHaveValue('0.5');
  await correction.getByLabel('Reason for correction').fill('broker paid 8.50');
  await correction.getByLabel('Cash proceeds').fill('8.50');
  await correction.getByRole('button',{name:'Preview cash in lieu',exact:true}).click();
  await expect(correction.getByRole('region',{name:'Fraction allocation and realized result'})).toContainText('realized result 5.166667');
  const replaced=page.waitForResponse(r=>r.url().endsWith('/replace-cash-in-lieu') && r.request().method()==='POST');
  await correction.getByRole('button',{name:'Save corrected cash in lieu'}).click();
  await page.getByRole('alertdialog').getByRole('button',{name:'Accept changed gains'}).click();
  const replacement=await(await replaced).json();
  await expect(correction).toBeHidden();
  await page.goto(`/app/transactions?transaction_id=${p.splitID}`);
  await page.getByRole('button',{name:'Reverse split…',exact:true}).click();
  await page.getByLabel('Reason for reversal').fill('withdraw split');
  await page.getByRole('button',{name:'Review and reverse'}).click();
  await expect(page.getByRole('alertdialog')).toContainText('later');
  await page.keyboard.press('Escape');
  await page.goto(`/app/transactions?transaction_id=${replacement.replacement.transaction.id}`);
  await page.getByRole('button',{name:'Reverse cash in lieu…',exact:true}).click();
  await page.getByLabel('Reason for reversal').fill('fraction payment withdrawn');
  await page.getByRole('button',{name:'Review and reverse'}).click();
  await page.getByRole('button',{name:'Accept changed gains'}).click();
  await expect(page.getByRole('alertdialog')).toBeHidden();
  await page.goto(`/app/transactions?transaction_id=${p.splitID}`);
  await page.getByRole('button',{name:'Reverse split…',exact:true}).click();
  await page.getByLabel('Reason for reversal').fill('withdraw split');
  await page.getByRole('button',{name:'Review and reverse'}).click();
  await expect(page.getByRole('alertdialog')).toBeHidden();
  const positions=await apiJSON<{positions:Array<{account_id:number;quantity_value:string;quantity_scale:number}>}>(page,'GET','/api/v1/investments/positions');
  const position=positions.positions.find(a=>a.account_id===p.holding.id)!;
  expect(BigInt(position.quantity_value)).toBe(3n*10n**BigInt(position.quantity_scale));
});

test('a backdated mobile cash-in-lieu entry reviews the FIFO gain it changes',async({page})=>{
  const p=await setup(page,true);
  await p.trade('sell',10,'2','3400');
  const form=await entry(page,p,20);
  await form.getByRole('button',{name:'Preview cash in lieu',exact:true}).click();
  await expect(form.getByRole('region',{name:'Fraction allocation and realized result'})).toContainText('Basis 2.00');
  await form.getByRole('button',{name:'Record cash in lieu',exact:true}).click();
  const review=page.getByRole('alertdialog');
  await expect(review).toContainText('basis 14.00 → 20.00');
  await expect(review).toContainText('gain 20.00 → 14.00');
  await review.getByRole('button',{name:'Accept changed gains'}).click();
  await expect(form).toBeHidden();
  const gains=await apiJSON<{realized:Array<{commodity_id:number;realized_gain_value:string;realized_gain_scale:number}>}>(page,'GET','/api/v1/investments/gains');
  expect(gains.realized.filter(g=>g.commodity_id===p.instrument.commodity_id).map(g=>(BigInt(g.realized_gain_value)*100n/10n**BigInt(g.realized_gain_scale)).toString()).sort()).toEqual(['1400','600']);
});


test('a specific-lot cash-in-lieu correction changes currency without hidden old choices or misleading gain units',async({page})=>{
  const p=await setup(page);
  const currencies=await apiJSON<{currencies:Array<{id:number;code:string}>}>(page,'GET','/api/v1/currencies');
  const originalCode=currencies.currencies.find(c=>c.id===p.currencyID)!.code;
  const targetCode=originalCode==='EUR' ? 'USD' : 'EUR';
  const targetID=await ensureCurrency(page,p.csrfToken,targetCode,targetCode==='USD' ? 'US Dollar' : 'Euro');
  const targetCash=await apiJSON<{id:number}>(page,'POST','/api/v1/accounts',p.csrfToken,{name:`CIL ${targetCode} cash ${Date.now()}`,account_class:'asset',account_kind:'brokerage_cash',default_commodity_id:targetID,allows_postings:true,opened_on:ago(60),effective_from:ago(60)});
  await apiJSON(page,'POST','/api/v1/investments/buy',p.csrfToken,{transaction_date:ago(40),commodity_id:p.instrument.commodity_id,holding_account_id:p.holding.id,cash_account_id:targetCash.id,quantity_value:'1',quantity_scale:0,cash_amount_value:'2000',cash_amount_scale:2,cash_commodity_id:targetID});
  const form=await entry(page,p);
  await form.getByLabel('Cost-basis method').selectOption('specific_lot');
  await form.getByLabel(/quantity to dispose \(available:/).fill('0.5');
  await form.getByRole('button',{name:'Preview cash in lieu',exact:true}).click();
  const posted=page.waitForResponse(r=>r.url().endsWith('/api/v1/investments/cash-in-lieu') && r.request().method()==='POST');
  await form.getByRole('button',{name:'Record cash in lieu',exact:true}).click();
  const original=await(await posted).json();
  await expect(form).toBeHidden();
  await page.goto(`/app/transactions?transaction_id=${original.transaction.id}`);
  await page.getByRole('button',{name:'Correct cash in lieu',exact:true}).click();
  const correction=page.getByRole('dialog',{name:'Correct cash in lieu',exact:true});
  await correction.getByLabel('Reason for correction').fill('cash and fraction belong to the other currency position');
  await correction.getByLabel('Received into').selectOption(String(targetCash.id));
  await correction.getByLabel('Settlement currency').selectOption(String(targetID));
  await correction.getByLabel(/quantity to dispose \(available: 1.5\)/).fill('0.5');
  await correction.getByLabel('Cash proceeds').fill('8.50');
  await correction.getByRole('button',{name:'Preview cash in lieu',exact:true}).click();
  await expect(correction.getByRole('region',{name:'Fraction allocation and realized result'})).toContainText(`realized result 1.833334 ${targetCode}`);
  await correction.getByRole('button',{name:'Save corrected cash in lieu'}).click();
  const review=page.getByRole('alertdialog');
  await expect(review).toContainText(`basis 3.333333 ${originalCode} → 6.666666 ${targetCode}`);
  await expect(review).toContainText(`gain 4.666667 ${originalCode} → 1.833334 ${targetCode}`);
  await review.getByRole('button',{name:'Accept changed gains'}).click();
  await expect(correction).toBeHidden();
  // Both cost-currency positions share one holding and instrument. The
  // overview must give them distinct identities and remain usable afterward.
  await page.goto('/app/investments');
  await expect(page.getByRole('button',{name:'Record buy',exact:true})).toBeVisible();
  const positionRows=page.getByRole('button').filter({hasText:p.name});
  await expect(positionRows).toHaveCount(2);
  for (let index=0;index<2;index++) {
    await positionRows.nth(index).click();
    const detail=page.getByRole('complementary',{name:'Lot detail'});
    await expect(detail.getByText('Original qty',{exact:true})).toHaveCount(1);
    await detail.getByRole('button',{name:'Close lot detail'}).click();
  }
});
