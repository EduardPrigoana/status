let instances = [];
let stats = {};
let currentHours = 24;
let eventSource = null;
let expandedGroups = new Set();

function connectSSE() {
    if (eventSource) {
        eventSource.close();
    }

    eventSource = new EventSource('/api/stream');

    eventSource.onopen = function() {
        updateConnectionStatus(true);
    };

    eventSource.onmessage = function(e) {
        try {
            const data = JSON.parse(e.data);
            instances = data.instances;
            stats = data.stats;
            renderUI();
            updateConnectionStatus(true);
        } catch (error) {
            console.error('Error parsing SSE data:', error);
        }
    };

    eventSource.onerror = function(e) {
        console.error('SSE error:', e);
        updateConnectionStatus(false);
        eventSource.close();
        setTimeout(connectSSE, 5000);
    };
}

function updateConnectionStatus(connected) {
    const dot = document.getElementById('connection-dot');
    const status = document.getElementById('connection-status');
    
    if (connected) {
        dot.classList.remove('disconnected');
        status.textContent = 'Live';
    } else {
        dot.classList.add('disconnected');
        status.textContent = 'Reconnecting...';
    }
}

function renderUI() {
    const content = document.getElementById('content');
    
    document.getElementById('total-count').textContent = stats.total_instances || 0;
    document.getElementById('up-count').textContent = stats.up_instances || 0;
    document.getElementById('down-count').textContent = (stats.total_instances - stats.up_instances) || 0;
    document.getElementById('avg-uptime').textContent = (stats.avg_uptime || 0).toFixed(1) + '%';

    const apiInstances = instances.filter(i => i.instance_type === 'api');
    const uiInstances = instances.filter(i => i.instance_type === 'ui');

    let html = '';

    if (apiInstances.length > 0) {
        html += '<div class="section-title">API Instances</div>';
        html += renderSection(apiInstances);
    }

    if (uiInstances.length > 0) {
        html += '<div class="section-title">UI Instances</div>';
        html += renderSection(uiInstances);
    }

    content.innerHTML = html || '<div class="loading">No instances found</div>';
}

function renderSection(sectionInstances) {
    const groupsMap = {};
    const groupOrder = [];

    sectionInstances.forEach(instance => {
        if (!groupsMap[instance.group]) {
            groupsMap[instance.group] = {
                instances: [],
                order: instance.group_order
            };
            groupOrder.push(instance.group);
        }
        groupsMap[instance.group].instances.push(instance);
    });

    groupOrder.sort((a, b) => groupsMap[a].order - groupsMap[b].order);

    let html = '';

    groupOrder.forEach(groupName => {
        const groupData = groupsMap[groupName];
        const groupInstances = groupData.instances;
        const groupId = 'group-' + groupName.replace(/\s+/g, '-').toLowerCase().replace(/[^a-z0-9-]/g, '') + '-' + groupInstances[0].instance_type;
        const isExpanded = expandedGroups.has(groupId);

        html += '<div class="group">';
        html += '<div class="group-header" onclick="toggleGroup(\'' + groupId + '\')">';
        html += '<div class="group-title">';
        html += '<span>' + escapeHtml(groupName) + '</span>';
        html += '<span class="group-count">' + groupInstances.length + ' ' + (groupInstances.length === 1 ? 'service' : 'services') + '</span>';
        html += '</div>';
        html += '<svg class="chevron' + (isExpanded ? ' expanded' : '') + '" id="chevron-' + groupId + '" fill="none" stroke="currentColor" viewBox="0 0 24 24">';
        html += '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>';
        html += '</svg>';
        html += '</div>';
        
        if (isExpanded) {
            html += '<div class="group-content expanded" id="content-' + groupId + '">';
            
            groupInstances.sort((a, b) => a.index - b.index);
            
            groupInstances.forEach(instance => {
                html += renderInstance(instance);
            });
            html += '</div>';
        } else {
            html += '<div class="group-content" id="content-' + groupId + '"></div>';
        }
        html += '</div>';
    });

    return html;
}

function renderInstance(instance) {
    const uptime = instance.uptime || 0;
    const uptimeClass = uptime > 99 ? 'good' : uptime > 95 ? 'medium' : 'bad';
    const isUp = instance.last_check && instance.last_check.success;
    const statusClass = isUp ? 'up' : 'down';
    const statusText = isUp ? 'UP' : 'DOWN';

    const lastCheckTime = instance.last_check 
        ? formatRelativeTime(new Date(instance.last_check.timestamp))
        : 'Never';

    const histogramData = getHistogramData(instance.checks, currentHours);

    let html = '<div class="instance">';
    html += '<div class="instance-header">';
    html += '<div class="instance-left">';
    html += '<div class="instance-title">';
    html += '<div class="instance-number">' + instance.index + '</div>';
    html += '<div class="status-indicator ' + statusClass + '"></div>';
    html += '<div class="instance-url">' + escapeHtml(instance.url) + '</div>';
    html += '</div>';
    html += '<div class="instance-meta">';
    html += '<span>Uptime: <span class="uptime-value ' + uptimeClass + '">' + uptime.toFixed(2) + '%</span></span>';
    html += '<span>Avg: <span class="meta-value">' + instance.avg_response_time + 'ms</span></span>';
    html += '<span>Last: <span class="meta-value">' + lastCheckTime + '</span></span>';
    html += '</div>';
    html += '</div>';
    html += '<div class="instance-right">';
    html += '<div class="status-badge ' + statusClass + '">' + statusText + '</div>';
    html += '<button class="badge-embed" onclick="showBadgeModal(\'' + escapeHtml(instance.url).replace(/'/g, "\\'") + '\')">Badge</button>';
    html += '</div>';
    html += '</div>';
    html += '<div class="histogram-container">';
    html += '<div class="histogram-header">';
    html += '<span>' + formatTimeRange(currentHours) + '</span>';
    html += '<span>' + histogramData.successCount + ' up / ' + histogramData.totalCount + ' checks</span>';
    html += '</div>';
    html += '<div class="histogram">';
    html += histogramData.html;
    html += '</div>';
    html += '</div>';
    html += '</div>';

    return html;
}

function getHistogramData(checks, hours) {
    const maxBars = Math.min(hours, 100);
    let html = '';

    if (checks.length === 0) {
        for (let i = 0; i < maxBars; i++) {
            html += '<div class="histogram-bar empty"></div>';
        }
        return { html: html, successCount: 0, totalCount: 0 };
    }

    const checksInRange = checks.slice(-hours);
    const step = Math.max(1, Math.floor(checksInRange.length / maxBars));
    
    for (let i = checksInRange.length - 1; i >= 0; i -= step) {
        const check = checksInRange[i];
        const statusClass = check.success ? 'success' : 'error';
        const time = new Date(check.timestamp).toLocaleString();
        
        const tooltip = '<div class="tooltip">' +
            '<div class="tooltip-time">' + time + '</div>' +
            '<div class="tooltip-status">Status: <span>' + (check.status_code || 'N/A') + '</span></div>' +
            '<div class="tooltip-status">Response: <span>' + check.response_time + 'ms</span></div>' +
            '</div>';
        
        html = '<div class="histogram-bar ' + statusClass + '">' + tooltip + '</div>' + html;
    }

    while (html.split('histogram-bar').length - 1 < maxBars) {
        html = '<div class="histogram-bar empty"></div>' + html;
    }

    const successCount = checksInRange.filter(c => c.success).length;
    const totalCount = checksInRange.length;

    return { html: html, successCount: successCount, totalCount: totalCount };
}

function formatTimeRange(hours) {
    if (hours <= 24) return 'Last 24 hours';
    if (hours <= 168) return 'Last 7 days';
    return hours + ' hours';
}

function formatRelativeTime(date) {
    const seconds = Math.floor((new Date() - date) / 1000);
    
    if (seconds < 60) return seconds + 's ago';
    if (seconds < 3600) return Math.floor(seconds / 60) + 'm ago';
    if (seconds < 86400) return Math.floor(seconds / 3600) + 'h ago';
    return Math.floor(seconds / 86400) + 'd ago';
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function toggleGroup(groupId) {
    const content = document.getElementById('content-' + groupId);
    const chevron = document.getElementById('chevron-' + groupId);

    if (expandedGroups.has(groupId)) {
        expandedGroups.delete(groupId);
        content.classList.remove('expanded');
        chevron.classList.remove('expanded');
    } else {
        expandedGroups.add(groupId);
        
        const groupData = instances.filter(i => {
            const iGroupId = 'group-' + i.group.replace(/\s+/g, '-').toLowerCase().replace(/[^a-z0-9-]/g, '') + '-' + i.instance_type;
            return iGroupId === groupId;
        });
        
        groupData.sort((a, b) => a.index - b.index);
        
        let html = '';
        groupData.forEach(instance => {
            html += renderInstance(instance);
        });
        
        content.innerHTML = html;
        content.classList.add('expanded');
        chevron.classList.add('expanded');
    }
}

function showBadgeModal(url) {
    const modal = document.getElementById('badge-modal');
    const badgeUrl = window.location.origin + '/api/badge/' + encodeURIComponent(url);
    
    document.getElementById('badge-url').textContent = badgeUrl;
    document.getElementById('badge-markdown').textContent = '![Status](' + badgeUrl + ')';
    document.getElementById('badge-html').textContent = '<img src="' + badgeUrl + '" alt="Status">';
    document.getElementById('badge-preview').innerHTML = '<img src="' + badgeUrl + '" alt="Status">';
    
    modal.classList.add('show');
}

function closeBadgeModal() {
    document.getElementById('badge-modal').classList.remove('show');
}

document.getElementById('badge-modal').addEventListener('click', function(e) {
    if (e.target === this) {
        closeBadgeModal();
    }
});

document.querySelectorAll('.time-btn').forEach(btn => {
    btn.addEventListener('click', function() {
        document.querySelectorAll('.time-btn').forEach(b => b.classList.remove('active'));
        this.classList.add('active');
        currentHours = parseInt(this.dataset.hours);
        renderUI();
    });
});

connectSSE();

window.addEventListener('beforeunload', function() {
    if (eventSource) {
        eventSource.close();
    }
});