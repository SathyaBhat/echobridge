// Accounts page. Depends on common.js.

document.addEventListener('DOMContentLoaded', () => {
    requestLocalNetworkAccess();
    loadAccounts();
    setupProviderTabs();
    setupMastodonForm();
    setupBlueskyForm();
});

async function requestLocalNetworkAccess() {
    if (!navigator.permissions || !navigator.permissions.request) return;
    try {
        await navigator.permissions.request({ name: 'local-network-access' });
    } catch (_) {
        // API not supported in this browser version; PNA headers handle older Chrome.
    }
}

function setupProviderTabs() {
    const tabs = document.querySelectorAll('.provider-tabs button[data-provider]');
    tabs.forEach((tab) => {
        tab.addEventListener('click', () => {
            tabs.forEach((t) => t.setAttribute('aria-selected', String(t === tab)));
            document.querySelectorAll('form[data-panel]').forEach((f) =>
                f.classList.toggle('hidden', f.dataset.panel !== tab.dataset.provider));
        });
    });
}

async function loadAccounts() {
    const container = document.getElementById('accounts-list');
    try {
        const response = await fetch(`${API_BASE}/accounts`);
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const accounts = (await response.json()) || [];

        if (accounts.length === 0) {
            container.innerHTML = '<p class="hint">No accounts connected yet.</p>';
            return;
        }

        container.innerHTML = accounts.map((account) => {
            const p = platformInfo(account.provider);
            const where = account.instance_url ? ` · ${esc(account.instance_url)}` : '';
            return `
                <div class="account-item">
                    <span class="avatar avatar--${esc(account.provider)}">${p.icon}</span>
                    <div class="account-info">
                        <span class="account-name">${esc(account.display_name)}</span>
                        <span class="account-provider"><span class="status-dot" title="Connected"></span>${esc(p.label)}${where}</span>
                    </div>
                    <button type="button" class="btn btn-danger btn-sm" data-delete="${esc(account.id)}">Disconnect</button>
                </div>`;
        }).join('');

        container.querySelectorAll('[data-delete]').forEach((btn) =>
            btn.addEventListener('click', () => deleteAccount(btn.dataset.delete)));
    } catch (error) {
        container.innerHTML = '<p class="hint">Failed to load accounts.</p>';
        console.error('Failed to load accounts:', error);
    }
}

async function deleteAccount(id) {
    if (!confirm('Disconnect this account?')) return;

    try {
        const response = await fetch(`${API_BASE}/accounts/${encodeURIComponent(id)}`, { method: 'DELETE' });
        if (response.ok) {
            toast('Account disconnected', 'success');
            loadAccounts();
        } else {
            toast('Failed to disconnect account', 'error');
        }
    } catch (error) {
        toast(error.message, 'error');
        console.error('Delete error:', error);
    }
}

function setupBlueskyForm() {
    const form = document.getElementById('bluesky-form');

    form.addEventListener('submit', async (e) => {
        e.preventDefault();

        const handle = document.getElementById('bluesky-handle').value.trim();
        const appPassword = document.getElementById('bluesky-app-password').value.trim();
        if (!handle || !appPassword) {
            toast('Enter your handle and app password', 'error');
            return;
        }

        const submitBtn = form.querySelector('button[type="submit"]');
        setBusy(submitBtn, true, 'Connecting…');

        try {
            const response = await fetch(`${API_BASE}/accounts/bluesky/connect`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ handle, app_password: appPassword }),
            });
            const data = await response.json().catch(() => ({}));

            if (response.ok) {
                form.reset();
                toast('Bluesky account connected', 'success');
                loadAccounts();
            } else {
                toast(data.error || 'Failed to connect account', 'error');
            }
        } catch (error) {
            toast(error.message, 'error');
            console.error('Bluesky connect error:', error);
        } finally {
            setBusy(submitBtn, false);
        }
    });
}

function setupMastodonForm() {
    const form = document.getElementById('mastodon-form');

    form.addEventListener('submit', async (e) => {
        e.preventDefault();

        const instanceUrl = document.getElementById('instance-url').value.trim();
        if (!instanceUrl) {
            toast('Enter an instance URL', 'error');
            return;
        }

        const submitBtn = form.querySelector('button[type="submit"]');
        setBusy(submitBtn, true, 'Connecting…');

        try {
            const response = await fetch(`${API_BASE}/accounts/mastodon/auth`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ instance_url: instanceUrl }),
            });
            const data = await response.json().catch(() => ({}));

            if (data.auth_url) {
                window.location.href = data.auth_url;
                return; // keep the button busy while redirecting
            }
            toast(data.error || 'Failed to start Mastodon authorization', 'error');
        } catch (error) {
            toast(error.message, 'error');
            console.error('Auth error:', error);
        }
        setBusy(submitBtn, false);
    });
}
