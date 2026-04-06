const API_BASE = (window.ECHOBRIDGE_CONFIG && window.ECHOBRIDGE_CONFIG.apiBase) || '/api';

let mediaFiles = [];

document.addEventListener('DOMContentLoaded', () => {
    loadAccounts();
    setupCharacterCounter();
    setupMediaPreview();
    setupFormSubmit();
});

async function loadAccounts() {
    const container = document.getElementById('accounts-list');
    try {
        const response = await fetch(`${API_BASE}/accounts`);
        const accounts = await response.json();

        if (accounts.length === 0) {
            container.innerHTML = '<p class="no-accounts">No accounts connected. <a href="profile.html">Add an account</a></p>';
            return;
        }

        container.innerHTML = accounts.map(account => `
            <label class="account-checkbox">
                <input type="checkbox" name="account_ids" value="${account.id}" checked>
                ${account.display_name} <small>(${account.provider}${account.instance_url ? ' - ' + account.instance_url : ''})</small>
            </label>
        `).join('');
    } catch (error) {
        container.innerHTML = '<p class="no-accounts">Failed to load accounts</p>';
        console.error('Failed to load accounts:', error);
    }
}

function setupCharacterCounter() {
    const textarea = document.getElementById('content');
    const counter = document.getElementById('char-count');

    textarea.addEventListener('input', () => {
        counter.textContent = textarea.value.length;
    });
}

function setupMediaPreview() {
    const input = document.getElementById('media');
    const preview = document.getElementById('media-preview');

    input.addEventListener('change', () => {
        const files = Array.from(input.files);
        files.forEach(file => {
            if (!mediaFiles.find(f => f.name === file.name && f.size === file.size)) {
                mediaFiles.push(file);
            }
        });
        renderMediaPreviews();
    });
}

function renderMediaPreviews() {
    const preview = document.getElementById('media-preview');
    preview.innerHTML = '';

    mediaFiles.forEach((file, index) => {
        const item = document.createElement('div');
        item.className = 'media-preview-item';

        const isVideo = file.type.startsWith('video/');
        const mediaEl = document.createElement(isVideo ? 'video' : 'img');
        mediaEl.src = URL.createObjectURL(file);

        const removeBtn = document.createElement('button');
        removeBtn.type = 'button';
        removeBtn.className = 'remove-btn';
        removeBtn.textContent = '×';
        removeBtn.onclick = () => {
            mediaFiles.splice(index, 1);
            renderMediaPreviews();
        };

        item.appendChild(mediaEl);
        item.appendChild(removeBtn);
        preview.appendChild(item);
    });
}

function setupFormSubmit() {
    const form = document.getElementById('compose-form');
    const submitBtn = document.getElementById('submit-btn');

    form.addEventListener('submit', async (e) => {
        e.preventDefault();

        const content = document.getElementById('content').value.trim();
        if (!content) {
            alert('Please enter a message');
            return;
        }

        const checkedBoxes = document.querySelectorAll('input[name="account_ids"]:checked');
        const accountIds = Array.from(checkedBoxes).map(cb => cb.value);
        if (accountIds.length === 0) {
            alert('Please select at least one account');
            return;
        }

        submitBtn.disabled = true;
        submitBtn.ariaBusy = 'true';
        submitBtn.textContent = 'Posting...';

        try {
            let uploadedMediaIds = [];
            for (const file of mediaFiles) {
                const formData = new FormData();
                formData.append('file', file);

                const uploadRes = await fetch(`${API_BASE}/media/upload`, {
                    method: 'POST',
                    body: formData
                });

                if (!uploadRes.ok) {
                    throw new Error(`Failed to upload ${file.name}`);
                }

                const uploadData = await uploadRes.json();
                uploadedMediaIds.push(uploadData.id);
            }

            const response = await fetch(`${API_BASE}/posts`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    content,
                    media_ids: uploadedMediaIds,
                    account_ids: accountIds
                })
            });

            const result = await response.json();
            displayResults(result.results || []);

            document.getElementById('content').value = '';
            document.getElementById('char-count').textContent = '0';
            mediaFiles = [];
            renderMediaPreviews();
            document.getElementById('media').value = '';

        } catch (error) {
            alert('Error posting: ' + error.message);
            console.error('Post error:', error);
        } finally {
            submitBtn.disabled = false;
            submitBtn.ariaBusy = 'false';
            submitBtn.textContent = 'Post';
        }
    });
}

function displayResults(results) {
    const section = document.getElementById('results-section');
    const list = document.getElementById('results-list');

    section.classList.remove('hidden');
    list.innerHTML = results.map(r => `
        <div class="result-item ${r.success ? 'result-success' : 'result-error'}">
            <strong>${r.display_name}</strong> (${r.provider})
            ${r.success 
                ? (r.post_url ? `<br><a href="${r.post_url}" target="_blank">View post</a>` : ' - Posted!')
                : `<br>Error: ${r.error}`
            }
        </div>
    `).join('');
}
