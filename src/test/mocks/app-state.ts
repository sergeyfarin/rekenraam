import { readable } from "svelte/store";

const url = new URL("http://localhost/");

export const page = {
  url,
  params: {},
  route: { id: null as string | null },
  status: 200,
  error: null,
  data: {},
  form: null,
  state: {},
};

export const navigating = readable(null);
export const updated = readable(false);
