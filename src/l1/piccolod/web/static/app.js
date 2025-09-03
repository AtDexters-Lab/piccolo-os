// Piccolo OS Web UI Application
class PiccoloApp {
    constructor() {
        this.baseURL = window.location.origin;
        this.apps = [];
        this.health = null;
        this.currentView = 'apps';
        this.connectionStatus = 'connecting';
        
        this.init();
    }

    async init() {
        console.log('🏠 Initializing Piccolo OS Web UI...');
        
        try {
            // Check API connection and get version
            await this.checkConnection();
            
            // Load initial data
            await this.loadApps();
            
            // Show main app
            this.showMainApp();
            
            // Start periodic refresh
            this.startPeriodicRefresh();
            
        } catch (error) {
            console.error('Failed to initialize:', error);
            this.showError(error.message || 'Failed to connect to Piccolo OS API');
        }
    }

    async checkConnection() {
        try {
            const response = await fetch(`${this.baseURL}/version`);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            
            const data = await response.json();
            this.setConnectionStatus('connected');
            this.setVersion(data.version || 'unknown');
            
            console.log('✅ Connected to Piccolo OS API');
            return data;
        } catch (error) {
            this.setConnectionStatus('error');
            throw new Error(`Cannot connect to API: ${error.message}`);
        }
    }

    async loadApps() {
        try {
            const response = await fetch(`${this.baseURL}/api/v1/apps`);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            
            const data = await response.json();
            this.apps = data.data || [];
            this.renderApps();
            
            console.log(`📱 Loaded ${this.apps.length} apps`);
        } catch (error) {
            console.error('Failed to load apps:', error);
            this.showToast('Failed to load applications', 'error');
        }
    }

    async loadHealth() {
        try {
            const response = await fetch(`${this.baseURL}/api/v1/health`);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            
            const data = await response.json();
            this.health = data;
            this.renderHealth();
            
            console.log('❤️ Health data loaded');
        } catch (error) {
            console.error('Failed to load health:', error);
            this.showToast('Failed to load system health', 'error');
        }
    }

    setConnectionStatus(status) {
        this.connectionStatus = status;
        const statusEl = document.getElementById('connection-status');
        const dot = statusEl.querySelector('.status-dot');
        const text = statusEl.querySelector('span:last-child');
        
        // Update dot with Tailwind classes
        switch (status) {
            case 'connected':
                dot.className = 'w-2 h-2 rounded-full bg-green-500 status-dot';
                text.textContent = 'Connected';
                break;
            case 'connecting':
                dot.className = 'w-2 h-2 rounded-full bg-yellow-500 animate-pulse status-dot';
                text.textContent = 'Connecting...';
                break;
            case 'error':
                dot.className = 'w-2 h-2 rounded-full bg-red-500 status-dot';
                text.textContent = 'Connection Error';
                break;
        }
    }

    setVersion(version) {
        const versionEl = document.getElementById('version-badge');
        versionEl.textContent = `v${version}`;
    }

    showMainApp() {
        document.getElementById('loading').style.display = 'none';
        document.getElementById('error').style.display = 'none';
        document.getElementById('main-app').style.display = 'block';
    }

    showError(message) {
        document.getElementById('loading').style.display = 'none';
        document.getElementById('main-app').style.display = 'none';
        document.getElementById('error').style.display = 'flex';
        document.getElementById('error-message').textContent = message;
    }

    renderApps() {
        const container = document.getElementById('apps-grid');
        
        if (this.apps.length === 0) {
            container.innerHTML = `
                <div class="col-span-full text-center py-12">
                    <div class="text-6xl mb-4">📱</div>
                    <p class="text-xl text-gray-600 mb-2">No applications installed yet.</p>
                    <p class="text-gray-500">Install your first app to get started!</p>
                </div>
            `;
            return;
        }

        container.innerHTML = this.apps.map(app => this.createAppCard(app)).join('');
    }

    createAppCard(app) {
        const globalUrl = this.getAppGlobalUrl(app);
        const statusClass = app.status ? app.status.toLowerCase() : 'unknown';
        
        // Status styling
        const statusStyles = {
            running: 'bg-green-100 text-green-800',
            stopped: 'bg-gray-100 text-gray-800', 
            error: 'bg-red-100 text-red-800',
            unknown: 'bg-yellow-100 text-yellow-800'
        };
        
        return `
            <div class="bg-white rounded-xl shadow-sm border border-gray-200 hover:shadow-md transition-shadow duration-200">
                <div class="p-6">
                    <!-- App header -->
                    <div class="flex items-start justify-between mb-4">
                        <div class="flex-1">
                            <h3 class="text-lg font-semibold text-gray-900 mb-1">${this.escapeHtml(app.name)}</h3>
                            <p class="text-sm text-gray-500 font-mono">${this.escapeHtml(app.image || 'No image')}</p>
                        </div>
                        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${statusStyles[statusClass] || statusStyles.unknown}">
                            ${app.status || 'Unknown'}
                        </span>
                    </div>
                    
                    <!-- App details -->
                    <div class="space-y-3 mb-6">
                        ${app.subdomain ? `
                            <div class="flex items-center justify-between text-sm">
                                <span class="text-gray-500">Global URL:</span>
                                <a href="${globalUrl}" target="_blank" class="text-piccolo-500 hover:text-piccolo-600 font-mono text-xs truncate max-w-48 ml-2">
                                    ${globalUrl.replace('https://', '')}
                                </a>
                            </div>
                        ` : ''}
                        
                        
                        
                        <div class="flex items-center justify-between text-sm">
                            <span class="text-gray-500">Type:</span>
                            <span class="text-xs bg-gray-100 px-2 py-1 rounded">${app.type || 'user'}</span>
                        </div>
                    </div>

                    <!-- App actions -->
                    <div class="flex space-x-2">
                        ${app.status === 'running' ? `
                            <button onclick="piccolo.stopApp('${app.name}')" class="flex-1 bg-yellow-500 hover:bg-yellow-600 text-white px-3 py-2 rounded-lg text-sm font-medium transition-colors flex items-center justify-center space-x-1">
                                <span>⏹️</span>
                                <span>Stop</span>
                            </button>
                        ` : `
                            <button onclick="piccolo.startApp('${app.name}')" class="flex-1 bg-green-500 hover:bg-green-600 text-white px-3 py-2 rounded-lg text-sm font-medium transition-colors flex items-center justify-center space-x-1">
                                <span>▶️</span>
                                <span>Start</span>
                            </button>
                        `}
                        
                        <button onclick="piccolo.showAppDetails('${app.name}')" class="px-3 py-2 bg-gray-100 hover:bg-gray-200 text-gray-700 rounded-lg text-sm transition-colors">
                            ℹ️
                        </button>
                        
                        <button onclick="piccolo.uninstallApp('${app.name}')" class="px-3 py-2 bg-red-100 hover:bg-red-200 text-red-700 rounded-lg text-sm transition-colors">
                            🗑️
                        </button>
                    </div>
                </div>
            </div>
        `;
    }

    getAppGlobalUrl(app) {
        if (!app.subdomain) return '#';
        
        // Extract domain from current URL
        const currentDomain = window.location.hostname;
        
        if (currentDomain.includes('piccolospace.com')) {
            // We're accessing via global domain - construct app URL
            const userDomain = currentDomain; // e.g., myname.piccolospace.com
            return `https://${app.subdomain}.${userDomain}`;
        } else {
            // We're accessing locally - show what the global URL would be
            return `https://${app.subdomain}.yourname.piccolospace.com`;
        }
    }

    renderHealth() {
        const container = document.getElementById('health-dashboard');
        
        if (!this.health) {
            container.innerHTML = `
                <div class="flex justify-center py-12">
                    <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-piccolo-500"></div>
                </div>
            `;
            return;
        }

        const sections = [];
        
        // Overall status
        const statusStyles = {
            healthy: 'bg-green-100 text-green-800',
            degraded: 'bg-yellow-100 text-yellow-800',
            unhealthy: 'bg-red-100 text-red-800',
            unknown: 'bg-gray-100 text-gray-800'
        };
        
        sections.push(`
            <div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6">
                <h3 class="text-lg font-semibold text-gray-900 mb-4">Overall System Health</h3>
                <div class="space-y-4">
                    <div class="flex items-center justify-between p-4 bg-gray-50 rounded-lg">
                        <div>
                            <span class="text-sm font-medium text-gray-700">Status:</span>
                        </div>
                        <span class="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium ${statusStyles[this.health.overall] || statusStyles.unknown}">
                            ${this.health.overall || 'Unknown'}
                        </span>
                    </div>
                    ${this.health.summary ? `
                        <div class="p-4 bg-gray-50 rounded-lg">
                            <span class="text-sm font-medium text-gray-700">Summary:</span>
                            <p class="text-sm text-gray-600 mt-1">${this.escapeHtml(this.health.summary)}</p>
                        </div>
                    ` : ''}
                </div>
            </div>
        `);

        // Component health
        if (this.health.components) {
            sections.push(`
                <div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6">
                    <h3 class="text-lg font-semibold text-gray-900 mb-4">Component Health</h3>
                    <div class="space-y-3">
                        ${Object.entries(this.health.components).map(([name, component]) => `
                            <div class="flex items-center justify-between p-4 bg-gray-50 rounded-lg">
                                <div class="flex-1">
                                    <div class="font-medium text-gray-900">${this.escapeHtml(name)}</div>
                                    <div class="text-sm text-gray-600">${this.escapeHtml(component.message || 'No details')}</div>
                                </div>
                                <span class="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium ${statusStyles[component.status] || statusStyles.unknown}">
                                    ${component.status || 'Unknown'}
                                </span>
                            </div>
                        `).join('')}
                    </div>
                </div>
            `);
        }

        container.innerHTML = sections.join('');
    }

    async startApp(appName) {
        try {
            const response = await fetch(`${this.baseURL}/api/v1/apps/${appName}/start`, {
                method: 'POST'
            });
            
            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error?.message || `HTTP ${response.status}`);
            }
            
            await this.loadApps(); // Refresh
            this.showToast(`Started ${appName}`, 'success');
        } catch (error) {
            console.error('Failed to start app:', error);
            this.showToast(`Failed to start ${appName}: ${error.message}`, 'error');
        }
    }

    async stopApp(appName) {
        try {
            const response = await fetch(`${this.baseURL}/api/v1/apps/${appName}/stop`, {
                method: 'POST'
            });
            
            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error?.message || `HTTP ${response.status}`);
            }
            
            await this.loadApps(); // Refresh
            this.showToast(`Stopped ${appName}`, 'success');
        } catch (error) {
            console.error('Failed to stop app:', error);
            this.showToast(`Failed to stop ${appName}: ${error.message}`, 'error');
        }
    }

    async uninstallApp(appName) {
        if (!confirm(`Are you sure you want to uninstall "${appName}"? This action cannot be undone.`)) {
            return;
        }

        try {
            const response = await fetch(`${this.baseURL}/api/v1/apps/${appName}`, {
                method: 'DELETE'
            });
            
            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error?.message || `HTTP ${response.status}`);
            }
            
            await this.loadApps(); // Refresh
            this.showToast(`Uninstalled ${appName}`, 'success');
        } catch (error) {
            console.error('Failed to uninstall app:', error);
            this.showToast(`Failed to uninstall ${appName}: ${error.message}`, 'error');
        }
    }

    showAppDetails(appName) {
        const app = this.apps.find(a => a.name === appName);
        if (!app) return;
        
        alert(`App Details for "${appName}"\n\n${JSON.stringify(app, null, 2)}`);
        // TODO: Replace with proper modal
    }

    async installApp() {
        const textarea = document.getElementById('app-yaml');
        const yamlContent = textarea.value.trim();
        
        if (!yamlContent) {
            this.showToast('Please enter app.yaml content', 'warning');
            return;
        }

        // Show loading state
        const btn = document.querySelector('#install-dialog button[onclick="installApp()"]');
        const btnText = btn.querySelector('.install-btn-text');
        const btnSpinner = btn.querySelector('.install-btn-spinner');
        
        if (btn && btnText && btnSpinner) {
            btn.disabled = true;
            btnText.style.display = 'none';
            btnSpinner.style.display = 'inline-block';
        }

        try {
            const response = await fetch(`${this.baseURL}/api/v1/apps`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/x-yaml'
                },
                body: yamlContent
            });

            const data = await response.json();

            if (!response.ok) {
                throw new Error(data.error?.message || `HTTP ${response.status}`);
            }

            await this.loadApps(); // Refresh apps
            this.closeInstallDialog();
            this.showToast(`Successfully installed ${data.data?.name || 'application'}`, 'success');

        } catch (error) {
            console.error('Failed to install app:', error);
            this.showToast(`Failed to install app: ${error.message}`, 'error');
        } finally {
            // Reset button state
            if (btn && btnText && btnSpinner) {
                btn.disabled = false;
                btnText.style.display = 'inline';
                btnSpinner.style.display = 'none';
            }
        }
    }

    showInstallDialog() {
        document.getElementById('install-dialog').style.display = 'flex';
        document.getElementById('app-yaml').focus();
    }

    closeInstallDialog() {
        document.getElementById('install-dialog').style.display = 'none';
        document.getElementById('app-yaml').value = '';
    }

    showView(viewName) {
        // Update navigation with Tailwind classes
        document.querySelectorAll('.nav-btn').forEach(btn => {
            if (btn.dataset.view === viewName) {
                btn.className = 'nav-btn border-b-2 border-piccolo-500 text-piccolo-600 py-4 px-1 text-sm font-medium';
            } else {
                btn.className = 'nav-btn border-b-2 border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 py-4 px-1 text-sm font-medium transition-colors';
            }
        });

        // Update views
        document.querySelectorAll('.view').forEach(view => {
            view.classList.toggle('active', view.id === `${viewName}-view`);
        });

        this.currentView = viewName;

        // Load data for specific views
        if (viewName === 'health' && !this.health) {
            this.loadHealth();
        }
    }

    showToast(message, type = 'success') {
        const container = document.getElementById('toast-container');
        const toast = document.createElement('div');
        
        // Tailwind toast styles
        const baseClasses = 'bg-white border rounded-lg shadow-lg p-4 max-w-sm transform transition-all duration-300 ease-in-out';
        const typeClasses = {
            success: 'border-green-200 text-green-800',
            error: 'border-red-200 text-red-800',
            warning: 'border-yellow-200 text-yellow-800'
        };
        
        toast.className = `${baseClasses} ${typeClasses[type] || typeClasses.success}`;
        
        // Add icon and message
        const icons = {
            success: '✅',
            error: '❌',
            warning: '⚠️'
        };
        
        toast.innerHTML = `
            <div class="flex items-center space-x-2">
                <span class="text-lg">${icons[type] || icons.success}</span>
                <span class="text-sm font-medium">${this.escapeHtml(message)}</span>
            </div>
        `;

        // Slide in animation
        toast.style.transform = 'translateX(100%)';
        container.appendChild(toast);
        
        // Trigger animation
        setTimeout(() => {
            toast.style.transform = 'translateX(0)';
        }, 10);

        // Auto remove after 5 seconds
        setTimeout(() => {
            if (toast.parentNode) {
                toast.style.transform = 'translateX(100%)';
                setTimeout(() => {
                    if (toast.parentNode) {
                        toast.parentNode.removeChild(toast);
                    }
                }, 300);
            }
        }, 5000);
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    startPeriodicRefresh() {
        // Refresh apps every 30 seconds
        setInterval(() => {
            if (this.currentView === 'apps') {
                this.loadApps();
            }
        }, 30000);

        // Check connection every 10 seconds
        setInterval(async () => {
            try {
                await this.checkConnection();
            } catch (error) {
                console.warn('Connection check failed:', error);
            }
        }, 10000);
    }
}

// Global functions for HTML onclick handlers
window.showView = (view) => window.piccolo.showView(view);
window.showInstallDialog = () => window.piccolo.showInstallDialog();
window.closeInstallDialog = () => window.piccolo.closeInstallDialog();
window.installApp = () => window.piccolo.installApp();
window.refreshHealth = () => window.piccolo.loadHealth();

// Initialize the app when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    window.piccolo = new PiccoloApp();
});

console.log('🏠 Piccolo OS Web UI loaded');
