export type SplitDraft = {
  account_id: number | null;
  category_id: number | null;
  category_input: string;
  tag_id: number | null;
  tag_input: string;
  person_id: number | null;
  person_input: string;
  project_id: number | null;
  project_input: string;
  share_bps: number | null;
  amount: string;
  memo: string;
};

export function emptySplitDraft(): SplitDraft {
  return {
    account_id: null,
    category_id: null,
    category_input: "",
    tag_id: null,
    tag_input: "",
    person_id: null,
    person_input: "",
    project_id: null,
    project_input: "",
    share_bps: null,
    amount: "",
    memo: "",
  };
}
