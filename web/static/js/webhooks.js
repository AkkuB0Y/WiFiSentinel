/**
 * Webhooks UI Manager
 */

const SentinelWebhooks = (() => {
    let webhooks = [];

    // DOM Elements
    const modal = document.getElementById('settings-modal');
    const btnOpen = document.getElementById('settings-btn');
    const btnClose = document.getElementById('close-modal-btn');
    
    const listView = document.getElementById('webhooks-list-view');
    const formView = document.getElementById('webhook-form-view');
    const btnNew = document.getElementById('new-webhook-btn');
    const btnCancel = document.getElementById('cancel-webhook-btn');
    const form = document.getElementById('webhook-form');
    const listContainer = document.getElementById('webhooks-list');

    // Form inputs
    const fId = document.getElementById('wh-id');
    const fName = document.getElementById('wh-name');
    const fUrl = document.getElementById('wh-url');
    const fLat = document.getElementById('wh-latency');
    const fLoss = document.getElementById('wh-loss');
    const fSig = document.getElementById('wh-signal');
    const fCool = document.getElementById('wh-cooldown');
    const fRec = document.getElementById('wh-recovery');

    function init() {
        if (!modal) return;

        // Event listeners for modal
        btnOpen.addEventListener('click', openModal);
        btnClose.addEventListener('click', closeModal);
        modal.addEventListener('click', (e) => {
            if (e.target === modal) closeModal();
        });

        // Event listeners for views
        btnNew.addEventListener('click', () => showForm());
        btnCancel.addEventListener('click', showList);

        // Form submission
        form.addEventListener('submit', handleSave);
    }

    async function openModal() {
        modal.classList.add('active');
        showList();
        await loadWebhooks();
    }

    function closeModal() {
        modal.classList.remove('active');
        setTimeout(() => {
            showList(); // reset view for next time
        }, 300);
    }

    function showList() {
        listView.style.display = 'block';
        formView.style.display = 'none';
    }

    function showForm(webhook = null) {
        listView.style.display = 'none';
        formView.style.display = 'block';

        if (webhook) {
            fId.value = webhook.id;
            fName.value = webhook.name;
            fUrl.value = webhook.url;
            fLat.value = webhook.latency_threshold === 0 ? '' : webhook.latency_threshold;
            fLoss.value = webhook.packet_loss_threshold === 0 ? '' : webhook.packet_loss_threshold;
            fSig.value = webhook.signal_threshold === 0 ? '' : webhook.signal_threshold;
            fCool.value = webhook.cooldown_minutes;
            fRec.checked = webhook.notify_recovery;
        } else {
            form.reset();
            fId.value = '';
            fCool.value = 5;
            fRec.checked = true;
        }
    }

    async function loadWebhooks() {
        listContainer.innerHTML = '<div class="empty-state">Loading webhooks...</div>';
        try {
            webhooks = await SentinelAPI.getWebhooks();
            renderList();
        } catch (err) {
            listContainer.innerHTML = '<div class="empty-state">Error loading webhooks.</div>';
            console.error(err);
        }
    }

    function renderList() {
        if (!webhooks || webhooks.length === 0) {
            listContainer.innerHTML = '<div class="empty-state">No webhooks configured yet.</div>';
            return;
        }

        listContainer.innerHTML = webhooks.map(wh => `
            <div class="webhook-item">
                <div class="webhook-info">
                    <h3>${escapeHtml(wh.name)}</h3>
                    <p>${escapeHtml(wh.url)}</p>
                </div>
                <div class="webhook-actions">
                    <button class="wh-action-btn" onclick="SentinelWebhooks.testWebhook(${wh.id})">Test</button>
                    <button class="wh-action-btn" onclick="SentinelWebhooks.editWebhook(${wh.id})">Edit</button>
                    <button class="wh-action-btn delete" onclick="SentinelWebhooks.deleteWebhook(${wh.id})">Delete</button>
                </div>
            </div>
        `).join('');
    }

    async function handleSave(e) {
        e.preventDefault();
        
        const data = {
            name: fName.value.trim(),
            url: fUrl.value.trim(),
            latency_threshold: parseInt(fLat.value) || 0,
            packet_loss_threshold: parseFloat(fLoss.value) || 0,
            signal_threshold: parseInt(fSig.value) || 0,
            cooldown_minutes: parseInt(fCool.value) || 5,
            notify_recovery: fRec.checked
        };

        const id = fId.value;
        const btn = document.getElementById('save-webhook-btn');
        const origText = btn.textContent;
        btn.textContent = 'Saving...';
        btn.disabled = true;

        try {
            if (id) {
                await SentinelAPI.updateWebhook(id, data);
            } else {
                await SentinelAPI.createWebhook(data);
            }
            await loadWebhooks();
            showList();
        } catch (err) {
            alert('Failed to save webhook: ' + err.message);
        } finally {
            btn.textContent = origText;
            btn.disabled = false;
        }
    }

    async function testWebhook(id) {
        try {
            const res = await SentinelAPI.testWebhook(id);
            if (res && res.status === 'sent') {
                alert('Test payload sent successfully!');
            } else {
                alert('Test failed: ' + ((res && res.error) || 'unknown error'));
            }
        } catch (err) {
            alert('Failed to test webhook: ' + err.message);
        }
    }

    function editWebhook(id) {
        const wh = webhooks.find(w => w.id === id);
        if (wh) showForm(wh);
    }

    async function deleteWebhook(id) {
        if (!confirm('Are you sure you want to delete this webhook?')) return;
        
        try {
            await SentinelAPI.deleteWebhook(id);
            await loadWebhooks();
        } catch (err) {
            alert('Failed to delete webhook: ' + err.message);
        }
    }

    // Utility
    function escapeHtml(unsafe) {
        return (unsafe || '').toString()
             .replace(/&/g, "&amp;")
             .replace(/</g, "&lt;")
             .replace(/>/g, "&gt;")
             .replace(/"/g, "&quot;")
             .replace(/'/g, "&#039;");
    }

    return {
        init,
        testWebhook,
        editWebhook,
        deleteWebhook
    };
})();

document.addEventListener('DOMContentLoaded', SentinelWebhooks.init);
