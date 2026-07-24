import { Navigate, Route, Routes } from 'react-router-dom'
import { useAuth } from '@/store/auth'
import LoginPage from '@/pages/LoginPage'
import DashboardLayout from '@/components/DashboardLayout'
import ErrorBoundary from '@/components/ErrorBoundary'
import HomePage from '@/pages/HomePage'
import DomainsPage from '@/pages/DomainsPage'
import SubscriptionDetailPage from '@/pages/SubscriptionDetailPage'
import ServicePlansPage from '@/pages/ServicePlansPage'
import SettingsPage from '@/pages/SettingsPage'
import PlaceholderPage from '@/pages/PlaceholderPage'
import ToolPage from '@/pages/ToolPage'
import DomainFilesPage from '@/pages/DomainFilesPage'
import DomainSSLPage from '@/pages/DomainSSLPage'
import DomainSSHPage from '@/pages/DomainSSHPage'
import DomainStatsPage from '@/pages/DomainStatsPage'
import DomainPerformancePage from '@/pages/DomainPerformancePage'
import DomainComposerPage from '@/pages/DomainComposerPage'
import DomainPasswordProtectPage from '@/pages/DomainPasswordProtectPage'
import DomainAntivirusPage from '@/pages/DomainAntivirusPage'
import DomainCopyPage from '@/pages/DomainCopyPage'
import DomainCronPage from '@/pages/DomainCronPage'
import DomainLogsPage from '@/pages/DomainLogsPage'
import DomainDNSPage from '@/pages/DomainDNSPage'
import RedisPage from '@/pages/RedisPage'
import DomainConnectionPage from '@/pages/DomainConnectionPage'
import DomainDatabasesPage from '@/pages/DomainDatabasesPage'
import DomainFTPPage from '@/pages/DomainFTPPage'
import DomainPHPPage from '@/pages/DomainPHPPage'
import DomainBackupsPage from '@/pages/DomainBackupsPage'
import DomainGitPage from '@/pages/DomainGitPage'
import DomainWebServerPage from '@/pages/DomainWebServerPage'
import DomainLaravelPage from '@/pages/DomainLaravelPage'
import DomainWafPage from '@/pages/DomainWafPage'
import PHPExtensionsPage from '@/pages/PHPExtensionsPage'
import PackagesPage from '@/pages/PackagesPage'
import PackageDetailPage from '@/pages/PackageDetailPage'
import PHPVersionsPage from '@/pages/PHPVersionsPage'
import ToolsSettingsPage from '@/pages/ToolsSettingsPage'
import PanelUpdatePage from '@/pages/PanelUpdatePage'
import ServerOptimizePage from '@/pages/ServerOptimizePage'
import DNSTemplatePage from '@/pages/DNSTemplatePage'
import WordPressPage from '@/pages/WordPressPage'
import FirewallPage from '@/pages/FirewallPage'
import BackupManagementPage from '@/pages/BackupManagementPage'
import DomainWordPressPage from '@/pages/DomainWordPressPage'
import DomainSubdomainsPage from '@/pages/DomainSubdomainsPage'
import DomainAddonDomainsPage from '@/pages/DomainAddonDomainsPage'
import DomainAccessControlPage from '@/pages/DomainAccessControlPage'
import CustomerLoginPage from '@/pages/CustomerLoginPage'
import StatisticsPage from '@/pages/StatisticsPage'
import MonitoringPage from '@/pages/MonitoringPage'
import ServicesPage from '@/pages/ServicesPage'
import ComingSoonPage from '@/pages/ComingSoonPage'

function GuardedRoute({ children }: { children: React.ReactNode }) {
  const token = useAuth((s) => s.token)
  if (!token) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <ErrorBoundary>
    <Routes>
      <Route path="/login" element={<LoginPage />} />
        <Route path="/cp/login" element={<CustomerLoginPage />} />
        <Route path="/cp" element={<CustomerLoginPage />} />
      <Route
        path="/"
        element={
          <GuardedRoute>
            <DashboardLayout />
          </GuardedRoute>
        }
      >
        <Route index                       element={<HomePage />} />
        <Route path="domains"            element={<DomainsPage />} />
        <Route path="subscriptions"          element={<Navigate to="/domains" replace />} />
        <Route path="subscriptions/:id"      element={<SubscriptionDetailPage />} />
        <Route path="subscriptions/:id/connection"      element={<DomainConnectionPage />} />
        <Route path="subscriptions/:id/files"      element={<DomainFilesPage />} />
        <Route path="subscriptions/:id/databases" element={<DomainDatabasesPage />} />
        <Route path="subscriptions/:id/ftp"           element={<DomainFTPPage />} />
        <Route path="subscriptions/:id/php"           element={<DomainPHPPage />} />
        <Route path="subscriptions/:id/ssl"           element={<DomainSSLPage />} />
        <Route path="subscriptions/:id/ssh-access"    element={<DomainSSHPage />} />
        <Route path="subscriptions/:id/stats"    element={<DomainStatsPage />} />
        <Route path="subscriptions/:id/performance"    element={<DomainPerformancePage />} />
        <Route path="subscriptions/:id/composer"      element={<DomainComposerPage />} />
        <Route path="subscriptions/:id/password-protection"  element={<DomainPasswordProtectPage />} />
        <Route path="subscriptions/:id/imunify"       element={<DomainAntivirusPage />} />
        <Route path="subscriptions/:id/copy"       element={<DomainCopyPage />} />
        <Route path="subscriptions/:id/wordpress"     element={<DomainWordPressPage />} />
        <Route path="subscriptions/:id/subdomains"  element={<DomainSubdomainsPage />} />
        <Route path="subscriptions/:id/addon-domains" element={<DomainAddonDomainsPage />} />
        <Route path="subscriptions/:id/access-control" element={<DomainAccessControlPage />} />
        <Route path="subscriptions/:id/cron"          element={<DomainCronPage />} />
        <Route path="subscriptions/:id/logs"     element={<DomainLogsPage />} />
        <Route path="subscriptions/:id/dns"           element={<DomainDNSPage />} />
        <Route path="subscriptions/:id/redis"         element={<RedisPage />} />
        <Route path="subscriptions/:id/backups"      element={<DomainBackupsPage />} />
        <Route path="subscriptions/:id/git"           element={<DomainGitPage />} />
        <Route path="subscriptions/:id/laravel"       element={<DomainLaravelPage />} />
        <Route path="subscriptions/:id/web-server"    element={<DomainWebServerPage />} />
        <Route path="subscriptions/:id/waf"           element={<DomainWafPage />} />
        <Route path="system/php-modules"           element={<PHPExtensionsPage />} />
        <Route path="tools/packages"               element={<PackagesPage />} />
        <Route path="tools/packages/:id"           element={<PackageDetailPage />} />
        <Route path="tools/php-versions"           element={<PHPVersionsPage />} />
        <Route path="tools/services"               element={<ServicesPage />} />
        <Route path="tools/dns-template"           element={<DNSTemplatePage />} />
        <Route path="subscriptions/:id/:slug" element={<ToolPage />} />
        <Route path="service-plans"      element={<ServicePlansPage />} />

        <Route path="tools-settings" element={<ToolsSettingsPage />} />
        <Route path="tools/update" element={<PanelUpdatePage />} />
        <Route path="tools/optimize" element={<ServerOptimizePage />} />
        <Route path="statistics" element={<StatisticsPage />} />
        <Route path="extensions" element={<ComingSoonPage title="Extensions" icon="🧩" description="Third-party extension management for the panel" features={["Browse the marketplace", "One-click install and removal", "Version updates", "API integration", "Developer SDK"]} />} />
        <Route path="wordpress" element={<WordPressPage />} />
        <Route path="firewall" element={<FirewallPage />} />
        <Route path="backup-management" element={<BackupManagementPage />} />
        <Route path="monitoring" element={<MonitoringPage />} />

        <Route path="profile"          element={<SettingsPage />} />
        <Route path="change-password" element={<Navigate to="/profile" replace />} />
        <Route path="settings"         element={<Navigate to="/profile" replace />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
    </ErrorBoundary>
  )
}
