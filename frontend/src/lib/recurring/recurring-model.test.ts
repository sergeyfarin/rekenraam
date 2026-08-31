import { describe, expect, it } from 'vitest';
import { templatePatch, templateValues, scheduleKeys } from './recurring-model';
import type { RecurringTemplate } from '$lib/api/recurring';

const template: RecurringTemplate = {
 id:1, revision:1, name:'Salary', enabled:true,
 frequency:'monthly', interval_count:1, by_weekday:null, day_of_month:31,
 last_day_of_month:false, month_of_year:null, starts_on:'2026-01-31', ends_on:null,
 max_occurrences:null, lead_days:5, generate_from:'2026-08-30', transaction_kind:'ordinary',
 payee_id:3, payee_name:'Employer', description:'Salary', note_markdown:'Note', tag_ids:[4],
 postings:[
 {line_key:'cash',account_id:1,commodity_id:2,quantity_value:'900719925474099312345',quantity_scale:4,memo:'cash memo'},
 {line_key:'income',account_id:2,commodity_id:2,quantity_value:'-900719925474099312345',quantity_scale:4,memo:'income memo'}
 ], created_at:'2026-08-30T00:00:00Z',updated_at:'2026-08-30T00:00:00Z',archived_at:null,next_due_on:'2026-08-31'
};
describe('recurring shared editor boundary',()=>{
 it('preserves exact postings, tags, payee and notes without saving a transaction',()=>{
  const values=templateValues(template,'2026-08-31');
  const patch=templatePatch({name:'Renamed'},values,template);
  expect(patch.postings).toEqual(template.postings);
  expect(patch.tag_ids).toEqual([4]);expect(patch.payee_id).toBe(3);
  expect(patch.note_markdown).toBe('Note');
 });
 it('omits unchanged schedule fields so an amount edit preserves catch-up',()=>{
  const schedule=Object.fromEntries(scheduleKeys.map(key=>[key,template[key]]));
  const patch=templatePatch(schedule,templateValues(template,'2026-08-31'),template);
  for(const key of scheduleKeys)expect(patch).not.toHaveProperty(key);
 });
 it('sends explicit nulls when changing frequency and clearing limits',()=>{
  const patch=templatePatch({frequency:'weekly',day_of_month:null,by_weekday:2,ends_on:null,max_occurrences:null},templateValues(template,'2026-08-31'),{...template,ends_on:'2027-01-01',max_occurrences:6});
  expect(patch).toMatchObject({frequency:'weekly',day_of_month:null,by_weekday:2,ends_on:null,max_occurrences:null});
 });
});
