// Shared helpers: theme, escaping, toasts, history store.
// Loaded in <head> (not deferred) so the theme is applied before first paint.

(function applyStoredTheme() {
    const stored = localStorage.getItem('echobridge.theme');
    if (stored === 'light' || stored === 'dark') {
        document.documentElement.dataset.theme = stored;
    } else {
        document.documentElement.dataset.theme =
            window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }
})();

window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
    if (!localStorage.getItem('echobridge.theme')) {
        document.documentElement.dataset.theme = e.matches ? 'dark' : 'light';
    }
});

const API_BASE = (window.ECHOBRIDGE_CONFIG && window.ECHOBRIDGE_CONFIG.apiBase) || '/api';

const PLATFORMS = {
    mastodon: { label: 'Mastodon', icon: '🦣', limit: 500 },
    bluesky: { label: 'Bluesky', icon: '🦋', limit: 300 },
};

function esc(value) {
    return String(value ?? '').replace(/[&<>"']/g, (c) => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
}

// Only allow http(s) links in href attributes.
function safeUrl(url) {
    try {
        const u = new URL(url);
        return (u.protocol === 'http:' || u.protocol === 'https:') ? u.href : '';
    } catch (_) {
        return '';
    }
}

function platformInfo(provider) {
    return PLATFORMS[provider] || { label: provider, icon: '🔗', limit: null };
}

function renderResultItem(r) {
    const p = platformInfo(r.provider);
    const link = r.success && safeUrl(r.post_url)
        ? `<a href="${esc(safeUrl(r.post_url))}" target="_blank" rel="noopener">View post ↗</a>`
        : '';
    const sub = r.success ? link : esc(r.error || 'Unknown error');
    return `
        <div class="result-item ${r.success ? 'result-success' : 'result-error'}">
            <span class="avatar avatar--${esc(r.provider)}">${p.icon}</span>
            <div class="result-body">
                <div class="result-name">${esc(r.display_name)}</div>
                <div class="result-sub">${sub}</div>
            </div>
            <span class="badge ${r.success ? 'badge-ok' : 'badge-err'}">${r.success ? 'Posted' : 'Failed'}</span>
        </div>`;
}

function toast(message, type = 'info') {
    let host = document.getElementById('toasts');
    if (!host) {
        host = document.createElement('div');
        host.id = 'toasts';
        host.className = 'toasts';
        host.setAttribute('role', 'status');
        host.setAttribute('aria-live', 'polite');
        document.body.appendChild(host);
    }
    const el = document.createElement('div');
    el.className = `toast toast-${type}`;
    el.textContent = message;
    host.appendChild(el);
    setTimeout(() => el.remove(), type === 'error' ? 6000 : 3500);
}

function setBusy(button, busy, busyLabel) {
    if (busy) {
        button.dataset.label = button.innerHTML;
        button.disabled = true;
        button.innerHTML = `<span class="spinner"></span>${busyLabel ? ' ' + esc(busyLabel) : ''}`;
    } else {
        button.disabled = false;
        if (button.dataset.label !== undefined) button.innerHTML = button.dataset.label;
    }
}

function timeAgo(iso) {
    const diff = (Date.now() - new Date(iso).getTime()) / 1000;
    if (diff < 60) return 'just now';
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    if (diff < 86400 * 7) return `${Math.floor(diff / 86400)}d ago`;
    return new Date(iso).toLocaleDateString();
}

// ---------- Post history (stored by the backend) ----------

async function apiFetch(path, options) {
    const response = await fetch(`${API_BASE}${path}`, options);
    if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(body.error || `Request failed (HTTP ${response.status})`);
    }
    return response.status === 204 ? null : response.json();
}

const History = {
    list(limit = 100) {
        return apiFetch(`/posts?limit=${limit}`);
    },
    remove(id) {
        return apiFetch(`/posts/${encodeURIComponent(id)}`, { method: 'DELETE' });
    },
    clear() {
        return apiFetch('/posts', { method: 'DELETE' });
    },
};

const DRAFT_KEY = 'echobridge.draft';

document.addEventListener('DOMContentLoaded', () => {
    const btn = document.getElementById('theme-toggle');
    if (!btn) return;
    const render = () => {
        const dark = document.documentElement.dataset.theme === 'dark';
        btn.textContent = dark ? '☀️' : '🌙';
        btn.setAttribute('aria-label', dark ? 'Switch to light mode' : 'Switch to dark mode');
        btn.title = btn.getAttribute('aria-label');
    };
    btn.addEventListener('click', () => {
        const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
        document.documentElement.dataset.theme = next;
        localStorage.setItem('echobridge.theme', next);
        render();
    });
    render();
});
