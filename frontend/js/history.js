// History page. Depends on common.js. Posts are stored by the backend.

let filter = 'all';
let entries = [];

document.addEventListener('DOMContentLoaded', () => {
    load();

    document.querySelectorAll('[data-filter]').forEach((btn) => {
        btn.addEventListener('click', () => {
            filter = btn.dataset.filter;
            document.querySelectorAll('[data-filter]').forEach((b) =>
                b.setAttribute('aria-pressed', String(b === btn)));
            render();
        });
    });

    document.getElementById('clear-history').addEventListener('click', async () => {
        if (entries.length === 0) return;
        if (!confirm('Delete all post history? This cannot be undone.')) return;
        try {
            await History.clear();
            entries = [];
            render();
        } catch (error) {
            toast(error.message, 'error');
        }
    });

    document.getElementById('history-list').addEventListener('click', async (e) => {
        const btn = e.target.closest('button[data-action]');
        if (!btn) return;
        const entry = entries.find((x) => x.id === btn.dataset.id);
        if (!entry) return;

        if (btn.dataset.action === 'delete') {
            try {
                await History.remove(entry.id);
                entries = entries.filter((x) => x.id !== entry.id);
                render();
            } catch (error) {
                toast(error.message, 'error');
            }
        } else if (btn.dataset.action === 'reuse') {
            localStorage.setItem(DRAFT_KEY, JSON.stringify({ content: entry.content, accountIds: null }));
            window.location.href = 'index.html';
        } else if (btn.dataset.action === 'retry') {
            const failedIds = entry.results.filter((r) => !r.success).map((r) => r.account_id);
            localStorage.setItem(DRAFT_KEY, JSON.stringify({ content: entry.content, accountIds: failedIds }));
            window.location.href = 'index.html';
        }
    });
});

async function load() {
    const host = document.getElementById('history-list');
    host.innerHTML = '<div class="card empty">Loading…</div>';
    try {
        entries = (await History.list()) || [];
    } catch (error) {
        host.innerHTML = `<div class="card empty">Failed to load history. ${esc(error.message)}</div>`;
        return;
    }
    render();
}

function overallBadge(results) {
    const failed = results.filter((r) => !r.success).length;
    if (failed === 0) return '<span class="badge badge-ok">All posted</span>';
    if (failed === results.length) return '<span class="badge badge-err">Failed</span>';
    return `<span class="badge badge-warn">${failed} failed</span>`;
}

function render() {
    const host = document.getElementById('history-list');
    let shown = entries;
    if (filter === 'failed') {
        shown = entries.filter((e) => (e.results || []).some((r) => !r.success));
    }

    if (shown.length === 0) {
        host.innerHTML = `<div class="card empty">${
            filter === 'failed' ? 'No failed posts.' : 'No posts yet. <a href="index.html">Compose your first post</a>.'
        }</div>`;
        return;
    }

    host.innerHTML = shown.map((entry) => {
        const results = entry.results || [];
        const hasFailure = results.some((r) => !r.success);
        const media = entry.media_count ? ` · 📎 ${entry.media_count}` : '';
        return `
            <article class="card history-item">
                <div class="history-head">
                    <span title="${esc(new Date(entry.created_at).toLocaleString())}">${esc(timeAgo(entry.created_at))}${media}</span>
                    ${overallBadge(results)}
                </div>
                <div class="history-content">${esc(entry.content)}</div>
                <div class="result-list">${results.map(renderResultItem).join('')}</div>
                <div class="history-actions">
                    ${hasFailure ? `<button type="button" class="btn btn-sm" data-action="retry" data-id="${esc(entry.id)}">Retry failed</button>` : ''}
                    <button type="button" class="btn btn-ghost btn-sm" data-action="reuse" data-id="${esc(entry.id)}">Reuse</button>
                    <button type="button" class="btn btn-danger btn-sm" data-action="delete" data-id="${esc(entry.id)}" style="margin-left:auto">Delete</button>
                </div>
            </article>`;
    }).join('');
}
