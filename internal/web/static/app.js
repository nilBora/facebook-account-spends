// Modal management
function showModal() {
    document.getElementById('modal-backdrop').classList.add('show');
}

function hideModal() {
    document.getElementById('modal-backdrop').classList.remove('show');
    document.getElementById('modal-content').innerHTML = '';
}

function hideConfirmModal() {
    document.getElementById('confirm-modal').classList.remove('show');
}

// Confirm delete dialog
function confirmDelete(itemName, deleteUrl, targetSelector) {
    const modal = document.getElementById('confirm-modal');
    const itemNameEl = document.getElementById('confirm-item-name');
    const deleteBtn = document.getElementById('confirm-delete-btn');

    itemNameEl.textContent = itemName;

    const newDeleteBtn = deleteBtn.cloneNode(true);
    deleteBtn.parentNode.replaceChild(newDeleteBtn, deleteBtn);

    newDeleteBtn.addEventListener('click', function () {
        htmx.ajax('DELETE', deleteUrl, {
            target: targetSelector,
            swap: 'innerHTML',
        }).then(function () {
            hideConfirmModal();
        });
    });

    modal.classList.add('show');
}

// Close modal on escape
document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') {
        hideModal();
        hideConfirmModal();
    }
});

// Theme
if (!document.cookie.includes('theme=')) {
    if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
        document.documentElement.setAttribute('data-theme', 'dark');
    }
}

function toggleTheme() {
    const current = document.documentElement.getAttribute('data-theme') || '';
    const next = current === 'dark' ? '' : 'dark';
    if (next === '') {
        document.documentElement.removeAttribute('data-theme');
    } else {
        document.documentElement.setAttribute('data-theme', next);
    }
    fetch('/web/theme', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: 'theme=' + encodeURIComponent(next),
    });
}

// Close modal after successful HTMX swap into a table target
document.body.addEventListener('htmx:afterSwap', function (e) {
    if (e.detail.target && e.detail.target.id && e.detail.target.id.endsWith('-table')) {
        hideModal();
    }
});
