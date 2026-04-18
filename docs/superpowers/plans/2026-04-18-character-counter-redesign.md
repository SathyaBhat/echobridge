# Character Counter Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single character count display with per-platform inline pills showing remaining characters, color-coded orange (≤20% remaining) and red (over limit), for each checked platform.

**Architecture:** The `.char-count` div renders one pill per checked platform account. Pills are rebuilt on every textarea `input` event and every account checkbox `change`. Platform limits are hardcoded in a JS map keyed by provider name string (e.g. `"mastodon"`, `"bluesky"`).

**Tech Stack:** Vanilla JS, Pico CSS, plain CSS custom classes

---

## File Map

- **Modify:** `frontend/index.html` — replace `<span id="char-count">` with `<div id="char-count">` (pill container)
- **Modify:** `frontend/js/app.js` — rewrite `setupCharacterCounter()`, update post-submit reset
- **Modify:** `frontend/css/style.css` — add `.char-pill`, `.char-pill--warning`, `.char-pill--over` classes

---

### Task 1: Update HTML — replace char-count span with a div container

**Files:**
- Modify: `frontend/index.html:34-36`

- [ ] **Step 1: Replace the char-count markup**

Open `frontend/index.html`. Find this block (lines 34–36):

```html
                <div class="char-count">
                    <span id="char-count">0</span> characters
                </div>
```

Replace it with:

```html
                <div class="char-count" id="char-count"></div>
```

The `id` moves to the wrapping div. The inner span and the word "characters" are removed — JS will render pills inside this div.

- [ ] **Step 2: Commit**

```bash
git add frontend/index.html
git commit -m "refactor: replace char-count span with pill container div"
```

---

### Task 2: Add CSS pill classes

**Files:**
- Modify: `frontend/css/style.css`

- [ ] **Step 1: Update the existing `.char-count` rule and add pill classes**

Open `frontend/css/style.css`. Find the existing `.char-count` block:

```css
.char-count {
    font-size: 0.875rem;
    color: var(--muted-color);
    text-align: right;
    margin-top: -0.5rem;
    margin-bottom: 1rem;
}
```

Replace it with:

```css
.char-count {
    font-size: 0.875rem;
    text-align: right;
    margin-top: -0.5rem;
    margin-bottom: 1rem;
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    flex-wrap: wrap;
}

.char-pill {
    color: var(--muted-color);
}

.char-pill--warning {
    color: #e67e22;
}

.char-pill--over {
    color: var(--del-color);
    font-weight: 600;
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/css/style.css
git commit -m "feat: add char-pill CSS classes for character counter"
```

---

### Task 3: Rewrite setupCharacterCounter in JS

**Files:**
- Modify: `frontend/js/app.js:35-42` (the `setupCharacterCounter` function)
- Modify: `frontend/js/app.js:143` (the post-submit reset line)

**Background:** The existing `setupCharacterCounter()` just sets `counter.textContent = textarea.value.length`. The `loadAccounts()` function renders checkboxes like:

```html
<label class="account-checkbox">
  <input type="checkbox" name="account_ids" value="1" checked>
  My Account <small>(mastodon - mastodon.social)</small>
</label>
```

The checkbox `value` is a numeric account ID. The provider name is in the `<small>` text, but it's cleaner to read it from the account data already loaded. The approach: store the provider on the checkbox element itself using a `data-provider` attribute, then read it in the counter.

- [ ] **Step 1: Add data-provider attribute to checkbox rendering in loadAccounts**

In `frontend/js/app.js`, find the `loadAccounts` function's `container.innerHTML` assignment (around line 23):

```javascript
        container.innerHTML = accounts.map(account => `
            <label class="account-checkbox">
                <input type="checkbox" name="account_ids" value="${account.id}" checked>
                ${account.display_name} <small>(${account.provider}${account.instance_url ? ' - ' + account.instance_url : ''})</small>
            </label>
        `).join('');
```

Replace it with:

```javascript
        container.innerHTML = accounts.map(account => `
            <label class="account-checkbox">
                <input type="checkbox" name="account_ids" value="${account.id}" data-provider="${account.provider}" checked>
                ${account.display_name} <small>(${account.provider}${account.instance_url ? ' - ' + account.instance_url : ''})</small>
            </label>
        `).join('');
```

The only change is adding `data-provider="${account.provider}"` to the checkbox input.

- [ ] **Step 2: Rewrite setupCharacterCounter**

Find the existing `setupCharacterCounter` function (lines 35–42):

```javascript
function setupCharacterCounter() {
    const textarea = document.getElementById('content');
    const counter = document.getElementById('char-count');

    textarea.addEventListener('input', () => {
        counter.textContent = textarea.value.length;
    });
}
```

Replace it with:

```javascript
const PLATFORM_LIMITS = {
    mastodon: 500,
    bluesky: 300,
};

const PLATFORM_ICONS = {
    mastodon: '🦣',
    bluesky: '🦋',
};

function updateCharCounter() {
    const textarea = document.getElementById('content');
    const counter = document.getElementById('char-count');
    const typed = textarea.value.length;

    const checkboxes = document.querySelectorAll('input[name="account_ids"]:checked');
    const providers = [...new Set(
        Array.from(checkboxes)
            .map(cb => cb.dataset.provider)
            .filter(p => p && PLATFORM_LIMITS[p])
    )];

    if (providers.length === 0) {
        counter.innerHTML = '';
        return;
    }

    counter.innerHTML = providers.map(provider => {
        const limit = PLATFORM_LIMITS[provider];
        const icon = PLATFORM_ICONS[provider] || provider;
        const remaining = limit - typed;
        let cls = 'char-pill';
        if (remaining < 0) {
            cls += ' char-pill--over';
        } else if (remaining <= Math.floor(limit * 0.2)) {
            cls += ' char-pill--warning';
        }
        return `<span class="${cls}">${icon} ${remaining}</span>`;
    }).join('');
}

function setupCharacterCounter() {
    const textarea = document.getElementById('content');
    textarea.addEventListener('input', updateCharCounter);

    document.getElementById('accounts-list').addEventListener('change', (e) => {
        if (e.target.matches('input[name="account_ids"]')) {
            updateCharCounter();
        }
    });
}
```

Note: `PLATFORM_LIMITS`, `PLATFORM_ICONS`, and `updateCharCounter` are defined at module scope so the post-submit reset can call `updateCharCounter()` too.

- [ ] **Step 3: Update the post-submit reset**

Find the reset block after a successful post (around line 142–146):

```javascript
            document.getElementById('content').value = '';
            document.getElementById('char-count').textContent = '0';
            mediaFiles = [];
```

Replace the `char-count` reset line:

```javascript
            document.getElementById('content').value = '';
            updateCharCounter();
            mediaFiles = [];
```

- [ ] **Step 4: Commit**

```bash
git add frontend/js/app.js
git commit -m "feat: per-platform character counter pills with color coding"
```

---

### Task 4: Manual verification

**No automated tests exist for the frontend JS** (no test runner is set up). Verify manually:

- [ ] **Step 1: Build and run**

```bash
make run
```

Open `http://localhost:8080` in a browser.

- [ ] **Step 2: Verify with accounts connected**

With at least one Mastodon and one Bluesky account checked:
- Counter row shows `🦣 500` and `🦋 300` at 0 typed chars (muted color)
- Type 440 characters → Mastodon pill shows `🦣 60` (muted), Bluesky shows `🦋 -140` (red, bold)
- Type exactly 400 chars → Mastodon shows `🦣 100` (muted, 100 = exactly 20% of 500), Bluesky shows `🦋 -100` (red)
- Type 401 chars → Mastodon shows `🦣 99` (orange, since 99 < 100)

- [ ] **Step 3: Verify checkbox toggling**

- Uncheck Bluesky account → Bluesky pill disappears immediately
- Re-check → Bluesky pill reappears with correct count
- Uncheck all accounts → counter row is empty

- [ ] **Step 4: Verify post-submit reset**

Submit a post → after success, counter resets correctly (pills reflect 0 typed chars against current checked platforms)

- [ ] **Step 5: Commit if any fixes needed**

```bash
git add frontend/js/app.js frontend/css/style.css frontend/index.html
git commit -m "fix: <describe what needed fixing>"
```
