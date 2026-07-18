import { createRouter, createWebHistory } from 'vue-router'

import { buildGuard } from './guards.js'
import { ensureAdminSession } from './api/client.js'
import { ensureUserSession } from './stores/session.js'
import SiteLayout from './views/SiteLayout.vue'
import LoginView from './views/LoginView.vue'
import RegisterView from './views/RegisterView.vue'
import WorkspaceView from './views/WorkspaceView.vue'
import WorkspaceLayout from './views/WorkspaceLayout.vue'
import OldPhotoRestorationView from './views/OldPhotoRestorationView.vue'
const VideoConversationWorkspaceView = () => import('./views/VideoConversationWorkspaceView.vue')
import VirtualTryOnWorkspaceView from './views/VirtualTryOnWorkspaceView.vue'
import NovelVideoWorkspaceView from './views/NovelVideoWorkspaceView.vue'
import AICommerceWorkspaceView from './views/AICommerceWorkspaceView.vue'
import MomentsMarketingWorkspaceView from './views/MomentsMarketingWorkspaceView.vue'
import ArticleImagesWorkspaceView from './views/ArticleImagesWorkspaceView.vue'
import CoupleAlbumWorkspaceView from './views/CoupleAlbumWorkspaceView.vue'
import CoupleAlbumDetailView from './views/CoupleAlbumDetailView.vue'
import CoupleAlbumShareView from './views/CoupleAlbumShareView.vue'
import WorksView from './views/WorksView.vue'
import WorksShareView from './views/WorksShareView.vue'
import AssetsView from './views/AssetsView.vue'
import PricingView from './views/PricingView.vue'
import CheckoutAlipayView from './views/CheckoutAlipayView.vue'
import ContactView from './views/ContactView.vue'
import TermsView from './views/TermsView.vue'
import PrivacyView from './views/PrivacyView.vue'
import AlgorithmDisclosureView from './views/AlgorithmDisclosureView.vue'
import ContentReportView from './views/ContentReportView.vue'
import AccountView from './views/AccountView.vue'
import AdminLayout from './views/AdminLayout.vue'
import AdminLoginView from './views/AdminLoginView.vue'
import AdminDashboardView from './views/AdminDashboardView.vue'
import AdminSettingsView from './views/AdminSettingsView.vue'
import AdminModelDetailView from './views/AdminModelDetailView.vue'
import AdminPromptTemplatesView from './views/AdminPromptTemplatesView.vue'
import AdminInspirationRecommendationsView from './views/AdminInspirationRecommendationsView.vue'
import AdminVideoStylePresetsView from './views/AdminVideoStylePresetsView.vue'
import AdminCoupleAlbumOptionsView from './views/AdminCoupleAlbumOptionsView.vue'
import AdminSystemSettingsView from './views/AdminSystemSettingsView.vue'
import AdminCommerceCategoriesView from './views/AdminCommerceCategoriesView.vue'
import AdminSystemLogsView from './views/AdminSystemLogsView.vue'
import AdminSystemResourcesView from './views/AdminSystemResourcesView.vue'
import AdminCustomerServiceView from './views/AdminCustomerServiceView.vue'
import AdminAnnouncementsView from './views/AdminAnnouncementsView.vue'
import AdminInvitesView from './views/AdminInvitesView.vue'
import AdminGenerationsView from './views/AdminGenerationsView.vue'
import AdminVideoGenerationsView from './views/AdminVideoGenerationsView.vue'
import AdminContentReviewsView from './views/AdminContentReviewsView.vue'
import AdminContentReportsView from './views/AdminContentReportsView.vue'
import AdminAlgorithmComplianceView from './views/AdminAlgorithmComplianceView.vue'
import AdminIncidentsView from './views/AdminIncidentsView.vue'
import AdminUsersView from './views/AdminUsersView.vue'
import AdminPackagesView from './views/AdminPackagesView.vue'
import AdminFinanceOrdersView from './views/AdminFinanceOrdersView.vue'
import AdminPermissionsView from './views/AdminPermissionsView.vue'
import AdminForbiddenView from './views/AdminForbiddenView.vue'

const router = createRouter({
  history: createWebHistory(),
  scrollBehavior(to) {
    if (to.hash) {
      return {
        el: to.hash,
        behavior: 'smooth'
      }
    }

    return { top: 0 }
  },
  routes: [
    {
      path: '/',
      component: SiteLayout,
      children: [
        { path: '', redirect: '/workspace' },
        { path: 'login', component: LoginView },
        { path: 'register', component: RegisterView },
        { path: 'works', component: WorksView, meta: { auth: 'user' } },
        { path: 'works/share', component: WorksShareView },
        { path: 'assets', component: AssetsView, meta: { auth: 'user' } },
        { path: 'couple-albums/share/:token', component: CoupleAlbumShareView },
        { path: 'pricing', component: PricingView },
        { path: 'checkout/alipay/return', component: CheckoutAlipayView, meta: { auth: 'user' } },
        { path: 'checkout/alipay/:order_number', component: CheckoutAlipayView, meta: { auth: 'user' } },
        { path: 'contact', component: ContactView },
        { path: 'terms', component: TermsView },
        { path: 'privacy', component: PrivacyView },
        { path: 'algorithm-disclosure', component: AlgorithmDisclosureView },
        { path: 'content-report', component: ContentReportView },
        { path: 'account', component: AccountView, meta: { auth: 'user' } }
      ]
    },
    {
      path: '/workspace',
      component: WorkspaceLayout,
      children: [
        { path: '', component: WorkspaceView },
        { path: 'image-to-image', redirect: '/workspace' },
        { path: 'old-photo-restoration', component: OldPhotoRestorationView, meta: { auth: 'user' } },
        { path: 'video', component: VideoConversationWorkspaceView, meta: { auth: 'user' } },
        { path: 'virtual-try-on', component: VirtualTryOnWorkspaceView, meta: { auth: 'user' } },
        { path: 'novel-video', component: NovelVideoWorkspaceView, meta: { auth: 'user' } },
        { path: 'ai-commerce', component: AICommerceWorkspaceView, meta: { auth: 'user' } },
        { path: 'moments-marketing', component: MomentsMarketingWorkspaceView, meta: { auth: 'user' } },
        { path: 'article-images', component: ArticleImagesWorkspaceView, meta: { auth: 'user' } },
        { path: 'couple-album', component: CoupleAlbumWorkspaceView, meta: { auth: 'user' } },
        { path: 'childhood-dream-album', component: CoupleAlbumWorkspaceView, meta: { auth: 'user' } },
        { path: 'couple-album/:id', component: CoupleAlbumDetailView, meta: { auth: 'user' } }
      ]
    },
    {
      path: '/admin/login',
      component: AdminLoginView
    },
    {
      path: '/admin',
      component: AdminLayout,
      meta: { auth: 'admin' },
      children: [
        { path: '', component: AdminDashboardView, meta: { adminPermission: 'dashboard.read' } },
        { path: 'settings', component: AdminSettingsView, meta: { adminPermission: 'settings.image.read' } },
        { path: 'settings/models/:id', component: AdminModelDetailView, meta: { adminPermission: 'settings.image.read' } },
        { path: 'prompt-templates', component: AdminPromptTemplatesView, meta: { adminPermission: 'prompt_templates.read' } },
        { path: 'inspiration-recommendations', component: AdminInspirationRecommendationsView, meta: { adminPermission: 'inspiration_recommendations.read' } },
        { path: 'video-style-presets', component: AdminVideoStylePresetsView, meta: { adminPermission: 'video_style_presets.read' } },
        { path: 'couple-album-options', component: AdminCoupleAlbumOptionsView, meta: { adminPermission: 'couple_album_options.read' } },
        { path: 'system-settings', component: AdminSystemSettingsView, meta: { adminPermission: 'system_settings.read' } },
		{ path: 'ecommerce-categories', component: AdminCommerceCategoriesView, meta: { adminPermission: 'system_settings.read' } },
        { path: 'system-logs', component: AdminSystemLogsView, meta: { adminPermission: 'system_logs.read' } },
        { path: 'system-resources', component: AdminSystemResourcesView, meta: { adminPermission: 'system_resources.read' } },
        { path: 'customer-service', component: AdminCustomerServiceView, meta: { adminPermission: 'customer_service.read' } },
        { path: 'announcements', component: AdminAnnouncementsView, meta: { adminPermission: 'announcements.read' } },
        { path: 'invites', component: AdminInvitesView, meta: { adminPermission: 'invites.read' } },
        { path: 'generations', component: AdminGenerationsView, meta: { adminPermission: 'generations.read' } },
        { path: 'video-generations', component: AdminVideoGenerationsView, meta: { adminPermission: 'generations.read' } },
        { path: 'content-reviews', component: AdminContentReviewsView, meta: { adminPermission: 'content_reviews.read' } },
        { path: 'content-reports', component: AdminContentReportsView, meta: { adminPermission: 'content_reports.read' } },
        { path: 'algorithm-compliance', component: AdminAlgorithmComplianceView, meta: { adminPermission: 'algorithm_compliance.read' } },
        { path: 'incidents', component: AdminIncidentsView, meta: { adminPermission: 'algorithm_incidents.read' } },
        { path: 'users', component: AdminUsersView, meta: { adminPermission: 'users.read' } },
        { path: 'packages', component: AdminPackagesView, meta: { adminPermission: 'packages.read' } },
        { path: 'finance-orders', component: AdminFinanceOrdersView, meta: { adminPermission: 'finance_orders.read' } },
        { path: 'permissions', component: AdminPermissionsView, meta: { adminPermission: 'admin_users.read' } },
        { path: 'forbidden', component: AdminForbiddenView }
      ]
    }
  ]
})

router.beforeEach(buildGuard({
  ensureUser: ensureUserSession,
  ensureAdmin: ensureAdminSession
}))

export default router
