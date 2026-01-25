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

<main>
  <div class="bx--grid">
    <div class="bx--row">
      <div class="bx--col-lg-8 bx--col-md-8 bx--col-sm-4">
        <h1 class="bx--type-productive-heading-05">Rekenraam</h1>
        <p class="bx--type-body-long-02">Personal finance tracking with a local-first database.</p>
      </div>
    </div>

    <div class="bx--row">
      <div class="bx--col-lg-6 bx--col-md-8 bx--col-sm-4">
        <form class="bx--form-item" onsubmit={greet}>
          <div class="bx--form-item">
            <label class="bx--label" for="greet-input">Name</label>
            <input
              id="greet-input"
              class="bx--text-input"
              placeholder="Enter a name..."
              bind:value={name}
            />
          </div>
          <div class="bx--form-item">
            <button class="bx--btn bx--btn--primary" type="submit">Greet</button>
          </div>
        </form>
        <p class="bx--type-body-short-01">{greetMsg}</p>
      </div>
    </div>

    <div class="bx--row">
      <div class="bx--col-lg-6 bx--col-md-8 bx--col-sm-4">
        <div class="bx--tile">
          <h2 class="bx--type-productive-heading-03">Database</h2>
          <div class="bx--form-item">
            <button class="bx--btn bx--btn--secondary" type="button" onclick={checkDbHealth}>
              Check DB Health
            </button>
          </div>
          <p class="bx--type-body-short-01">{dbHealth}</p>
        </div>
      </div>

      <div class="bx--col-lg-6 bx--col-md-8 bx--col-sm-4">
        <div class="bx--tile">
          <h2 class="bx--type-productive-heading-03">Schema</h2>
          <div class="bx--form-item">
            <button class="bx--btn bx--btn--secondary" type="button" onclick={checkSchemaVersion}>
              Check Schema Version
            </button>
          </div>
          <p class="bx--type-body-short-01">{schemaVersion}</p>
        </div>
      </div>
    </div>
  </div>
</main>
