const API_BASE = '/api';

document.addEventListener('DOMContentLoaded', () => {
    loadAccounts();
    setupMastodonForm();
});

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

function setupMastodonForm() {
    const form = document.getElementById('mastodon-form');

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
