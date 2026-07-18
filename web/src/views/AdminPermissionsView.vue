<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const admins = ref([])
const roles = ref([])
const permissions = ref([])
const selectedAdminId = ref('')
const selectedRoleId = ref('')
const selectedAdminRoleIds = ref([])
const selectedPermissionIds = ref([])
const errorMessage = ref('')
const successMessage = ref('')

const adminForm = reactive({
  username: '',
  display_name: '',
  password: '',
  role_ids: []
})

const roleForm = reactive({
  code: '',
  name: '',
  description: ''
})

const selectedAdmin = computed(() => admins.value.find((item) => item.id === Number(selectedAdminId.value)))
const selectedRole = computed(() => roles.value.find((item) => item.id === Number(selectedRoleId.value)))

watch(selectedAdmin, (admin) => {
  selectedAdminRoleIds.value = (admin?.roles ?? []).map((role) => role.id)
})

watch(selectedRole, (role) => {
  selectedPermissionIds.value = (role?.permissions ?? []).map((permission) => permission.id)
})

async function load() {
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const [adminPayload, rolePayload] = await Promise.all([
      api.listAdminAccounts(),
      api.listAdminRoles()
    ])
    admins.value = adminPayload.items ?? []
    roles.value = rolePayload.items ?? []
    permissions.value = rolePayload.permissions ?? []
    if (!selectedAdminId.value && admins.value[0]) selectedAdminId.value = `${admins.value[0].id}`
    if (!selectedRoleId.value && roles.value[0]) selectedRoleId.value = `${roles.value[0].id}`
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function createAdmin() {
  errorMessage.value = ''
  try {
    await api.createAdminAccount({
      ...adminForm,
      role_ids: adminForm.role_ids.map(Number)
    })
    adminForm.username = ''
    adminForm.display_name = ''
    adminForm.password = ''
    adminForm.role_ids = []
    successMessage.value = '管理员已创建'
    await load()
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function createRole() {
  errorMessage.value = ''
  try {
    await api.createAdminRole({ ...roleForm })
    roleForm.code = ''
    roleForm.name = ''
    roleForm.description = ''
    successMessage.value = '角色已创建'
    await load()
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function saveAdminRoles() {
  if (!selectedAdmin.value) return
  try {
    await api.updateAdminAccountRoles(selectedAdmin.value.id, {
      role_ids: selectedAdminRoleIds.value.map(Number)
    })
    successMessage.value = '管理员角色已保存'
    await load()
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function saveRolePermissions() {
  if (!selectedRole.value) return
  try {
    await api.updateAdminRolePermissions(selectedRole.value.id, {
      permission_ids: selectedPermissionIds.value.map(Number)
    })
    successMessage.value = '角色权限已保存'
    await load()
  } catch (error) {
    errorMessage.value = error.message
  }
}

onMounted(load)
</script>

<template>
  <section>
    <div class="section-heading">
      <div>
        <p class="eyebrow">RBAC</p>
        <h1>管理员与权限</h1>
      </div>
      <button class="secondary-button" type="button" @click="load">刷新</button>
    </div>

    <div class="split-grid">
      <form class="form-panel" @submit.prevent="createAdmin">
        <h2 class="panel-title">新增管理员</h2>
        <label class="field-label">账号</label>
        <input v-model="adminForm.username" class="text-input" />
        <label class="field-label">显示名</label>
        <input v-model="adminForm.display_name" class="text-input" />
        <label class="field-label">初始密码</label>
        <input v-model="adminForm.password" class="text-input" type="password" />
        <label class="field-label">角色</label>
        <label v-for="role in roles" :key="role.id" class="check-row">
          <input v-model="adminForm.role_ids" type="checkbox" :value="role.id" />
          <span>{{ role.name }} · {{ role.code }}</span>
        </label>
        <button class="primary-button" type="submit">新增管理员</button>
      </form>

      <form class="form-panel" @submit.prevent="createRole">
        <h2 class="panel-title">新增角色</h2>
        <label class="field-label">编码</label>
        <input v-model="roleForm.code" class="text-input" />
        <label class="field-label">名称</label>
        <input v-model="roleForm.name" class="text-input" />
        <label class="field-label">描述</label>
        <textarea v-model="roleForm.description" class="text-area" rows="4" />
        <button class="primary-button" type="submit">新增角色</button>
      </form>
    </div>

    <div class="split-grid">
      <div class="table-panel">
        <h2 class="panel-title">管理员角色</h2>
        <label class="field-label">管理员</label>
        <ClickSelect
          v-model="selectedAdminId"
          :options="admins.map((admin) => ({ value: admin.id, label: `${admin.display_name || admin.username} · ${admin.username}` }))"
          class="text-input"
          aria-label="管理员"
        />
        <div class="check-list">
          <label v-for="role in roles" :key="role.id" class="check-row">
            <input v-model="selectedAdminRoleIds" type="checkbox" :value="role.id" />
            <span>{{ role.name }} · {{ role.code }}</span>
          </label>
        </div>
        <button class="secondary-button" type="button" @click="saveAdminRoles">保存角色</button>
      </div>

      <div class="table-panel">
        <h2 class="panel-title">角色权限</h2>
        <label class="field-label">角色</label>
        <ClickSelect
          v-model="selectedRoleId"
          :options="roles.map((role) => ({ value: role.id, label: `${role.name} · ${role.code}` }))"
          class="text-input"
          aria-label="角色"
        />
        <div class="permission-grid">
          <label v-for="permission in permissions" :key="permission.id" class="check-row">
            <input v-model="selectedPermissionIds" type="checkbox" :value="permission.id" />
            <span>{{ permission.name }} · {{ permission.code }}</span>
          </label>
        </div>
        <button class="secondary-button" type="button" @click="saveRolePermissions">保存权限</button>
      </div>
    </div>

    <p v-if="successMessage" class="page-status">{{ successMessage }}</p>
    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
  </section>
</template>
