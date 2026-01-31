<script lang="ts">
  let name = "";
  let greetMsg = "";
  let dbHealth = "(unknown)";
  let schemaVersion = "(unknown)";

  async function greet(event: Event) {
    event.preventDefault();
    try {
      const { invoke } = await import("@tauri-apps/api/core");
      greetMsg = await invoke("greet", { name });
    } catch (e) {
      greetMsg = `error: ${String(e)}`;
    }
  }

  async function checkDbHealth() {
    try {
      const { invoke } = await import("@tauri-apps/api/core");
      const res = await invoke<string>("db_health");
      dbHealth = res;
    } catch (e) {
      dbHealth = `error: ${String(e)}`;
    }
  }

  async function checkSchemaVersion() {
    try {
      const { invoke } = await import("@tauri-apps/api/core");
      const res = await invoke<number>("get_schema_version");
      schemaVersion = String(res);
    } catch (e) {
      schemaVersion = `error: ${String(e)}`;
    }
  }
</script>

<main class="page">
  <div class="page-grid container">
    <div class="page-row">
      <div class="page-col">
        <h1 class="page-title">Rekenraam</h1>
        <p class="page-subtitle">Personal finance tracking with a local-first database.</p>
      </div>
    </div>

    <div class="page-row">
      <div class="page-col">
        <form onsubmit={greet}>
          <div class="form-field">
            <label class="label" for="greet-input">Name</label>
            <input
              id="greet-input"
              class="input"
              placeholder="Enter a name..."
              bind:value={name}
            />
          </div>
          <div class="form-field">
            <button class="btn btn-primary" type="submit">Greet</button>
          </div>
        </form>
        <p class="text-sm text-muted">{greetMsg}</p>
      </div>
    </div>

    <div class="page-row">
      <div class="page-col">
        <div class="card">
          <h2 class="section-title">Database</h2>
          <div class="form-field">
            <button class="btn btn-secondary" type="button" onclick={checkDbHealth}>
              Check DB Health
            </button>
          </div>
          <p class="text-sm text-muted">{dbHealth}</p>
        </div>
      </div>

      <div class="page-col">
        <div class="card">
          <h2 class="section-title">Schema</h2>
          <div class="form-field">
            <button class="btn btn-secondary" type="button" onclick={checkSchemaVersion}>
              Check Schema Version
            </button>
          </div>
          <p class="text-sm text-muted">{schemaVersion}</p>
        </div>
      </div>
    </div>
  </div>
</main>
