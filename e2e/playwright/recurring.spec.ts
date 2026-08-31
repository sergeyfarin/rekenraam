import { expect, test, type Page } from '@playwright/test';
import { apiJSON, csrfTokenFor } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

async function templateForm(page:Page,name:string,account:string){
 await page.getByRole('button',{name:'New template',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'Recurring review'});
 await dialog.getByLabel('Template name').fill(name);
 await dialog.getByRole('combobox',{name:'Frequency',exact:true}).selectOption('daily');
 await dialog.getByLabel('Create drafts days ahead').fill('0');
 await dialog.getByLabel('Description',{exact:true}).fill(name);
 await dialog.getByRole('combobox',{name:'Account',exact:true}).selectOption({label:account});
 await dialog.getByLabel('Amount',{exact:true}).fill('-12.34');
 await dialog.getByPlaceholder('Search categories').click();
 await dialog.locator('ul[role="listbox"] button[role="option"]').first().click();
 await expect(dialog.getByRole('region',{name:'Next five scheduled dates'}).locator('time')).toHaveCount(5);
 return dialog;
}
async function archive(page:Page,id:number){await apiJSON(page,'POST',`/api/v1/recurring/templates/${id}/archive`,await csrfTokenFor(page),{});}

test('[acceptance] recurring template creates a reviewable draft, edits and posts it explicitly',async({page})=>{
 const {csrfToken,currencyID}=await readyForLedger(page);
 const name=`Recurring salary ${Date.now()}`;
 const account=await createCashAccount(page,csrfToken,`${name} account`,currencyID);
 const tag=await apiJSON<{id:number;name:string}>(page,'POST','/api/v1/tags',csrfToken,{name:`Recurring tag ${Date.now()}`},[201]);
 await page.goto('/app/recurring');
 const dialog=await templateForm(page,name,account.name);
 await dialog.getByRole('checkbox',{name:tag.name,exact:true}).check();
 await expectNoAccessibilityViolations(page,'recurring template editor');
 await page.screenshot({path:'/tmp/r9-template.png',fullPage:true});
 await dialog.getByRole('button',{name:'Save template',exact:true}).click();
 await expect(dialog).not.toBeVisible();
 const list=await apiJSON<{templates:Array<{id:number;name:string}>}>(page,'GET','/api/v1/recurring/templates');
 const template=list.templates.find(t=>t.name===name)!;
 try{
 await page.getByRole('button',{name:'Templates',exact:true}).click();
 const card=page.getByRole('article',{name,exact:true});
 await card.getByRole('button',{name:'Generate due drafts'}).click();
 const due=page.getByRole('article',{name,exact:true});
 await expect(due.getByRole('button',{name:'Post draft',exact:true})).toBeVisible();
 const inbox=await apiJSON<{items:Array<{template_id:number;transaction_id:number}>}>(page,'GET','/api/v1/recurring/due');
 const id=inbox.items.find(i=>i.template_id===template.id)!.transaction_id;
 let draft=await apiJSON<{status:string;tag_ids:number[];journal_entries:Array<{postings:Array<{quantity_value:string}>}>}>(page,'GET',`/api/v1/transactions/${id}`);
 expect(draft.status).toBe('draft');expect(draft.tag_ids).toEqual([tag.id]);
 await due.getByRole('button',{name:'Edit draft'}).click();
 const edit=page.getByRole('dialog');
 await edit.getByLabel('Amount',{exact:true}).nth(0).fill('-15.67');
 await edit.getByLabel('Amount',{exact:true}).nth(1).fill('15.67');
 await edit.getByRole('button',{name:'Save transaction',exact:true}).click();
 await expect(edit).not.toBeVisible();
 await expect(due).toContainText('15.67');
 draft=await apiJSON(page,'GET',`/api/v1/transactions/${id}`);expect(draft.status).toBe('draft');expect(draft.tag_ids).toEqual([tag.id]);
 await due.getByRole('button',{name:'Post draft',exact:true}).click();
 await expect(page.getByRole('heading',{name:'Review before posting'})).toBeVisible();
 await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click();
 await expect(due).not.toBeVisible();
 const posted=await apiJSON<{status:string}>(page,'GET',`/api/v1/transactions/${id}`);expect(posted.status).toBe('posted');
 }finally{await archive(page,template.id);}
});

test('[acceptance] recurring drafts remain reachable after archive and mobile discard never regenerates',async({page})=>{
 await page.setViewportSize({width:390,height:844});
 const {csrfToken,currencyID}=await readyForLedger(page);
 const name=`Recurring mobile ${Date.now()}`;
 const account=await createCashAccount(page,csrfToken,`${name} account`,currencyID);
 await page.goto('/app/recurring');
 const dialog=await templateForm(page,name,account.name);
 await dialog.getByRole('button',{name:'Save template',exact:true}).click();
 await expect(dialog).not.toBeVisible();
 const list=await apiJSON<{templates:Array<{id:number;name:string}>}>(page,'GET','/api/v1/recurring/templates');
 const template=list.templates.find(t=>t.name===name)!;
 await page.getByRole('button',{name:'Templates',exact:true}).click();
 let card=page.getByRole('article',{name,exact:true});
 await card.getByRole('button',{name:'Generate due drafts'}).click();
 await expect(card.getByRole('button',{name:'Discard draft'})).toBeVisible();
 await page.getByRole('button',{name:'Templates',exact:true}).click();
 await card.getByRole('button',{name:'Scheduled occurrences'}).click();
 await card.getByRole('button',{name:'Skip occurrence',exact:true}).first().click();
 await page.getByRole('dialog').getByLabel('Reason for skipping').fill('Not needed next time');
 await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click();
 await expect(page.getByRole('dialog')).not.toBeVisible();
 await card.getByRole('button',{name:'Archive template',exact:true}).click();
 await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click();
 await expect(page.getByRole('dialog')).not.toBeVisible();
 await page.getByRole('button',{name:'Due for review',exact:true}).click();
 card=page.getByRole('article',{name,exact:true});
 await expect(card).toContainText('Archived template');
 await page.getByRole('button',{name:'Switch to dark theme'}).click();
 await expectNoAccessibilityViolations(page,'recurring mobile due inbox');
 await page.screenshot({path:'/tmp/r9-due-mobile.png',fullPage:true});
 await card.getByRole('button',{name:'Discard draft'}).click();
 await expectNoAccessibilityViolations(page,'recurring mobile discard confirmation');
 await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click();
 await expect(card).not.toBeVisible();
 const result=await apiJSON<{generated:number}>(page,'POST',`/api/v1/recurring/templates/${template.id}/run-now`,await csrfTokenFor(page),{});
 expect(result.generated).toBe(0);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});

test('recurring inbox has loading, empty and recoverable error states',async({page})=>{
 await readyForLedger(page);
 let fail=true;
 await page.route('**/api/v1/recurring/due*',async route=>{
  if(fail)await route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:{code:'RESOURCE_BUSY',message:'Busy'}})});
  else await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({items:[],next_cursor:null})});
 });
 await page.goto('/app/recurring');
 await expect(page.getByText('Recurring entries unavailable')).toBeVisible({timeout:20000});
 fail=false;
 await page.getByRole('button',{name:'Refresh',exact:true}).last().click();
 await expect(page.getByText('Nothing to review',{exact:true})).toBeVisible();
 await expectNoAccessibilityViolations(page,'recurring empty inbox');
});

async function seedDue(page:Page,name:string,accountID:number,currencyID:number,enabled=true){
 const categories=await apiJSON<{categories:Array<{id:number;category_type:string;allows_postings:boolean}>}>(page,'GET','/api/v1/categories');
 const category=categories.categories.find(c=>c.category_type==='expense'&&c.allows_postings)!;
 const {today}=await apiJSON<{today:string}>(page,'GET','/api/v1/recurring/summary');
 const template=await apiJSON<{id:number}>(page,'POST','/api/v1/recurring/templates',await csrfTokenFor(page),{
  name,enabled,frequency:'daily',starts_on:today,lead_days:0,description:name,
  postings:[{account_id:accountID,commodity_id:currencyID,quantity_value:'-1000',quantity_scale:2},{account_id:category.id,commodity_id:currencyID,quantity_value:'1000',quantity_scale:2}]
 },[201]);
 return template;
}

test('bulk review preserves completed posts when a later draft fails',async({page})=>{
 const {csrfToken,currencyID}=await readyForLedger(page);
 const name=`Bulk recurring ${Date.now()}`;
 const account=await createCashAccount(page,csrfToken,name,currencyID);
 const first=await seedDue(page,`${name} first`,account.id,currencyID);
 const second=await seedDue(page,`${name} second`,account.id,currencyID);
 try{
 for(const t of [first,second])await apiJSON(page,'POST',`/api/v1/recurring/templates/${t.id}/run-now`,await csrfTokenFor(page),{});
 const inbox=await apiJSON<{items:Array<{template_id:number;transaction_id:number}>}>(page,'GET','/api/v1/recurring/due');
 const secondID=inbox.items.find(i=>i.template_id===second.id)!.transaction_id;
 await page.goto('/app/recurring');
 const firstCard=page.getByRole('article',{name:`${name} first`,exact:true});
 const secondCard=page.getByRole('article',{name:`${name} second`,exact:true});
 await firstCard.getByRole('checkbox').check();await secondCard.getByRole('checkbox').check();
 await page.getByRole('button',{name:'Post selected drafts'}).click();
 await page.route(`**/api/v1/transactions/${secondID}/post`,route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:{code:'RESOURCE_BUSY',message:'Busy'}})}));
 await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click();
 await expect(firstCard).not.toBeVisible();
 const dialog=page.getByRole('dialog');
 await expect(dialog.getByText(`${name} first`,{exact:false})).toHaveCount(0);
 await expect(dialog.getByText(`${name} second`,{exact:false})).toBeVisible();
 await page.unroute(`**/api/v1/transactions/${secondID}/post`);
 await dialog.getByRole('button',{name:'Confirm',exact:true}).click();
 await expect(dialog).not.toBeVisible();await expect(secondCard).not.toBeVisible();
 const posted=await apiJSON<{status:string}>(page,'GET',`/api/v1/transactions/${secondID}`);expect(posted.status).toBe('posted');
 }finally{await archive(page,first.id);await archive(page,second.id);}
});

test('blocked recurring occurrence can be repaired and explicitly retried',async({page})=>{
 const {csrfToken,currencyID}=await readyForLedger(page);
 const name=`Blocked recurring ${Date.now()}`;
 const oldAccount=await createCashAccount(page,csrfToken,`${name} old`,currencyID);
 const replacement=await createCashAccount(page,csrfToken,`${name} replacement`,currencyID);
 const template=await seedDue(page,name,oldAccount.id,currencyID);
 try{
 await apiJSON(page,'POST',`/api/v1/accounts/${oldAccount.id}/close`,await csrfTokenFor(page),{});
 await apiJSON(page,'POST',`/api/v1/accounts/${oldAccount.id}/archive`,await csrfTokenFor(page),{});
 const run=await apiJSON<{blocked:number}>(page,'POST',`/api/v1/recurring/templates/${template.id}/run-now`,await csrfTokenFor(page),{});
 expect(run.blocked).toBe(1);
 await page.goto('/app/recurring');
 const card=page.getByRole('article',{name,exact:true});
 await expect(card).toContainText('No draft was created');
 await card.getByText('Diagnostic details').click();
 await expect(card.getByRole('button',{name:'Retry',exact:true})).toBeVisible();
 const saved=await apiJSON<{postings:Array<{account_id:number}>}>(page,'GET',`/api/v1/recurring/templates/${template.id}`);
 await apiJSON(page,'PATCH',`/api/v1/recurring/templates/${template.id}`,await csrfTokenFor(page),{postings:saved.postings.map(p=>({...p,account_id:p.account_id===oldAccount.id?replacement.id:p.account_id}))});
 await card.getByRole('button',{name:'Retry',exact:true}).click();
 await expect(card.getByRole('button',{name:'Post draft',exact:true})).toBeVisible();
 await card.getByRole('button',{name:'Discard draft'}).click();
 await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click();
 await expect(card).not.toBeVisible();
 const again=await apiJSON<{generated:number}>(page,'POST',`/api/v1/recurring/templates/${template.id}/run-now`,await csrfTokenFor(page),{});
 expect(again.generated).toBe(0);
 }finally{await archive(page,template.id);}
});

test('changed reconciliation impact stops posting and asks for a new review',async({page})=>{
 const {csrfToken,currencyID}=await readyForLedger(page);
 const name=`Changed impact ${Date.now()}`;
 const account=await createCashAccount(page,csrfToken,name,currencyID);
 const template=await seedDue(page,name,account.id,currencyID);
 try{
 await apiJSON(page,'POST',`/api/v1/recurring/templates/${template.id}/run-now`,await csrfTokenFor(page),{});
 const inbox=await apiJSON<{items:Array<{template_id:number;transaction_id:number}>}>(page,'GET','/api/v1/recurring/due');
 const id=inbox.items.find(i=>i.template_id===template.id)!.transaction_id;
 await page.goto('/app/recurring');
 await page.getByRole('article',{name,exact:true}).getByRole('button',{name:'Post draft',exact:true}).click();
 const dialog=page.getByRole('dialog');
 await expect(dialog.getByRole('heading',{name:'Review before posting'})).toBeVisible();
 // A checkpoint appears after the confirmation was prepared. The UI must
 // refuse the old approval before issuing any mutation.
 await page.route(`**/api/v1/transactions/${id}/post/reconciliation-impact`,route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({affected_checkpoints:[{checkpoint_id:999,account_id:account.id,account_label:account.name,commodity_id:currencyID,commodity_code:'USD',statement_date:'2026-08-31'}]})}));
 await dialog.getByRole('button',{name:'Confirm',exact:true}).click();
 await expect(dialog.getByRole('alert')).toContainText('Reconciliation impacts changed');
 await expect(dialog.getByRole('button',{name:'Confirm',exact:true})).toBeDisabled();
 const draft=await apiJSON<{status:string}>(page,'GET',`/api/v1/transactions/${id}`);expect(draft.status).toBe('draft');
 await dialog.getByRole('button',{name:'Cancel',exact:true}).click();
 await page.getByRole('article',{name,exact:true}).getByRole('button',{name:'Discard draft'}).click();
 await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click();
 }finally{await archive(page,template.id);}
});


test('editing a recurring template preserves payee, description and posting details', async ({page}) => {
 const {csrfToken,currencyID}=await readyForLedger(page);
 const name=`Template preservation ${Date.now()}`;
 const account=await createCashAccount(page,csrfToken,name,currencyID);
 const payee=await apiJSON<{id:number;name:string}>(page,'POST','/api/v1/payees',csrfToken,{name:`Payee ${Date.now()}`},[201]);
 const template=await seedDue(page,name,account.id,currencyID,false);
 try {
 await apiJSON(page,'PATCH',`/api/v1/recurring/templates/${template.id}`,csrfToken,{payee_id:payee.id});
 const before=await apiJSON<{postings:unknown[]}>(page,'GET',`/api/v1/recurring/templates/${template.id}`);
 await page.goto('/app/recurring');
 await page.getByRole('button',{name:'Templates',exact:true}).click();
 await page.getByRole('article',{name,exact:true}).getByRole('button',{name:'Edit template'}).click();
 const dialog=page.getByRole('dialog');
 const payeeInput=dialog.getByPlaceholder('Search or enter payee name');
 await expect(payeeInput).toHaveValue(payee.name);
 await dialog.getByLabel('Description',{exact:true}).fill(`${name} edited`);
 await payeeInput.fill('Changed payee');
 await expect(payeeInput).toHaveValue('Changed payee');
 await expect(dialog.getByLabel('Description',{exact:true})).toHaveValue(`${name} edited`);
 await payeeInput.fill(payee.name);
 await dialog.getByRole('option',{name:payee.name,exact:true}).click();
 await dialog.getByRole('button',{name:'Save template',exact:true}).click();
 await expect(dialog).not.toBeVisible();
 const after=await apiJSON<{payee_id:number;description:string;postings:unknown[]}>(page,'GET',`/api/v1/recurring/templates/${template.id}`);
 expect(after.payee_id).toBe(payee.id);
 expect(after.description).toBe(`${name} edited`);
 expect(after.postings).toEqual(before.postings);
 } finally { await archive(page,template.id); }
});
