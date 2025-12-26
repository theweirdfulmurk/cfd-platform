const API_BASE = 'http://localhost:8082/api/v1';

// State management
let jobs = [];
let autoRefreshInterval = null;

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    setupEventListeners();
    loadJobs();
    startAutoRefresh();
});

function setupEventListeners() {
    const form = document.getElementById('job-form');
    const fileInput = document.getElementById('input-file');
    const fileInfo = document.getElementById('file-info');

    // File input handler
    fileInput.addEventListener('change', (e) => {
        const file = e.target.files[0];
        if (file) {
            fileInfo.textContent = `📄 ${file.name} (${formatFileSize(file.size)})`;
            fileInfo.classList.add('has-file');
        } else {
            fileInfo.textContent = 'Файл не выбран';
            fileInfo.classList.remove('has-file');
        }
    });

    // Form submission
    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        await submitJob();
    });
}

async function submitJob() {
    const form = document.getElementById('job-form');
    const submitBtn = form.querySelector('button[type="submit"]');
    const btnText = submitBtn.querySelector('.btn-text');
    const btnLoader = submitBtn.querySelector('.btn-loader');

    try {
        // Disable button
        submitBtn.disabled = true;
        btnText.style.display = 'none';
        btnLoader.style.display = 'inline-block';

        const formData = new FormData(form);
        
        const response = await fetch(`${API_BASE}/jobs`, {
            method: 'POST',
            body: formData
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const job = await response.json();
        
        // Add to jobs list
        jobs.unshift(job);
        renderJobs();

        // Reset form
        form.reset();
        document.getElementById('file-info').textContent = 'Файл не выбран';
        document.getElementById('file-info').classList.remove('has-file');

        showNotification('✅ Задание создано успешно!', 'success');
        
    } catch (error) {
        console.error('Error submitting job:', error);
        showNotification('❌ Ошибка при создании задания', 'error');
    } finally {
        submitBtn.disabled = false;
        btnText.style.display = 'inline';
        btnLoader.style.display = 'none';
    }
}

async function loadJobs() {
    try {
        const response = await fetch(`${API_BASE}/jobs`);
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        jobs = await response.json();
        renderJobs();
    } catch (error) {
        console.error('Error loading jobs:', error);
        showNotification('⚠️ Ошибка загрузки заданий', 'warning');
    }
}

function renderJobs() {
    const jobsList = document.getElementById('jobs-list');
    
    if (jobs.length === 0) {
        jobsList.innerHTML = '<p class="empty-state">Нет заданий. Создайте первое задание выше.</p>';
        return;
    }

    jobsList.innerHTML = jobs.map(job => createJobCard(job)).join('');
}

function createJobCard(job) {
    const typeName = job.type === 'cfd' ? 'CFD (OpenFOAM)' : 'FEA (CalculiX)';
    const statusClass = `status-${job.status}`;
    const statusText = getStatusText(job.status);
    const createdAt = new Date(job.created_at).toLocaleString('ru-RU');
    const completedAt = job.completed_at ? new Date(job.completed_at).toLocaleString('ru-RU') : '-';
    
    let actionsHtml = '';
    if (job.status === 'completed') {
        actionsHtml = `
            <div class="job-actions">
                <button class="btn btn-primary btn-sm" onclick="downloadResults('${job.id}')">
                    📥 Скачать результаты
                </button>
            </div>
        `;
    }

    let errorHtml = '';
    if (job.error) {
        errorHtml = `
            <div class="job-detail">
                <span class="job-detail-label">Ошибка:</span>
                <span style="color: var(--danger);">${job.error}</span>
            </div>
        `;
    }

    return `
        <div class="job-card" data-job-id="${job.id}">
            <div class="job-header">
                <div class="job-info">
                    <h3>🔬 ${typeName}</h3>
                    <div class="job-meta">ID: ${job.id.substring(0, 8)}</div>
                </div>
                <span class="status-badge ${statusClass}">${statusText}</span>
            </div>
            <div class="job-details">
                <div class="job-detail">
                    <span class="job-detail-label">Входной файл:</span>
                    <span>${job.input_file}</span>
                </div>
                <div class="job-detail">
                    <span class="job-detail-label">Создано:</span>
                    <span>${createdAt}</span>
                </div>
                <div class="job-detail">
                    <span class="job-detail-label">Завершено:</span>
                    <span>${completedAt}</span>
                </div>
                ${errorHtml}
            </div>
            ${actionsHtml}
        </div>
    `;
}

function getStatusText(status) {
    const statusMap = {
        'submitted': 'Отправлено',
        'running': 'Выполняется',
        'completed': 'Завершено',
        'failed': 'Ошибка'
    };
    return statusMap[status] || status;
}

async function downloadResults(jobId) {
    try {
        window.open(`${API_BASE}/results?id=${jobId}`, '_blank');
        showNotification('📥 Загрузка результатов...', 'info');
    } catch (error) {
        console.error('Error downloading results:', error);
        showNotification('❌ Ошибка загрузки результатов', 'error');
    }
}

async function refreshJobs() {
    const btn = event.target;
    btn.disabled = true;
    btn.textContent = '⏳ Обновление...';
    
    await loadJobs();
    
    setTimeout(() => {
        btn.disabled = false;
        btn.textContent = '🔄 Обновить';
    }, 500);
}

function startAutoRefresh() {
    // Auto-refresh every 10 seconds
    autoRefreshInterval = setInterval(() => {
        loadJobs();
    }, 10000);
}

function showNotification(message, type = 'info') {
    // Simple console notification
    // You can implement a toast notification UI here
    console.log(`[${type.toUpperCase()}] ${message}`);
    
    // Optional: Show browser notification if supported
    if ('Notification' in window && Notification.permission === 'granted') {
        new Notification('CFD/FEA Platform', { body: message });
    }
}

function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

// Cleanup
window.addEventListener('beforeunload', () => {
    if (autoRefreshInterval) {
        clearInterval(autoRefreshInterval);
    }
});
