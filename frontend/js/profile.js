const API_BASE = (window.ECHOBRIDGE_CONFIG && window.ECHOBRIDGE_CONFIG.apiBase) || '/api';

document.addEventListener('DOMContentLoaded', () => {
    requestLocalNetworkAccess();
    loadAccounts();
    setupMastodonForm();
    setupBlueskyForm();
});

async function requestLocalNetworkAccess() {
    if (!navigator.permissions || !navigator.permissions.request) return;
    try {
        await navigator.permissions.request({ name: 'local-network-access' });
    } catch (_) {
        // API not supported in this browser version — PNA headers handle older Chrome
    }
}

async function loadAccounts() {
    const container = document.getElementById('accounts-list');
    try {
        const response = await fetch(`${API_BASE}/accounts`);
        const accounts = await response.json();

        if (accounts.length === 0) {
            container.innerHTML = '<p class="no-accounts">No accounts connected yet.</p>';
            return;
        }

        container.innerHTML = accounts.map(account => `
            <div class="account-item">
                <div class="account-info">
                    <span class="account-name">${account.display_name}</span>
                    <span class="account-provider">${account.provider}${account.instance_url ? ' (' + account.instance_url + ')' : ''}</span>
                </div>
                <button class="secondary outline" onclick="deleteAccount('${account.id}')">Disconnect</button>
            </div>
        `).join('');
    } catch (error) {
        container.innerHTML = '<p class="no-accounts">Failed to load accounts</p>';
        console.error('Failed to load accounts:', error);
    }
}

async function deleteAccount(id) {
    if (!confirm('Are you sure you want to disconnect this account?')) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/accounts/${id}`, {
            method: 'DELETE'
        });

        if (response.ok) {
            loadAccounts();
        } else {
            alert('Failed to disconnect account');
        }
    } catch (error) {
        alert('Error: ' + error.message);
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
            alert('Please enter your handle and app password');
            return;
        }

        const submitBtn = form.querySelector('button[type="submit"]');
        submitBtn.disabled = true;
        submitBtn.ariaBusy = 'true';

        try {
            const response = await fetch(`${API_BASE}/accounts/bluesky/connect`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ handle, app_password: appPassword })
            });

            const data = await response.json();

            if (response.ok) {
                form.reset();
                loadAccounts();
            } else {
                alert('Error: ' + (data.error || 'Failed to connect account'));
            }
        } catch (error) {
            alert('Error: ' + error.message);
            console.error('Bluesky connect error:', error);
        } finally {
            submitBtn.disabled = false;
            submitBtn.ariaBusy = 'false';
        }
    });
}

function setupMastodonForm() {

    form.addEventListener('submit', async (e) => {
        e.preventDefault();

        const instanceUrl = document.getElementById('instance-url').value.trim();
        if (!instanceUrl) {
            alert('Please enter an instance URL');
            return;
        }

        const submitBtn = form.querySelector('button[type="submit"]');
        submitBtn.disabled = true;
        submitBtn.ariaBusy = 'true';

        try {
            const response = await fetch(`${API_BASE}/accounts/mastodon/auth`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ instance_url: instanceUrl })
            });

            const data = await response.json();

            if (data.auth_url) {
                window.location.href = data.auth_url;
            } else if (data.error) {
                alert('Error: ' + data.error);
            }
        } catch (error) {
            alert('Error: ' + error.message);
            console.error('Auth error:', error);
        } finally {
            submitBtn.disabled = false;
            submitBtn.ariaBusy = 'false';
        }
    });
}
