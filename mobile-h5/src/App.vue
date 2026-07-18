<script>
import { api, getStoredAuthToken } from './api/client.js'

const presenceHeartbeatIntervalMS = 60_000
let presenceHeartbeatTimer = null

function pingPresence() {
  if (!getStoredAuthToken()) return
  api.pingPresence().catch(() => {})
}

function stopPresenceHeartbeat() {
  if (presenceHeartbeatTimer) {
    clearInterval(presenceHeartbeatTimer)
    presenceHeartbeatTimer = null
  }
}

function startPresenceHeartbeat() {
  stopPresenceHeartbeat()
  pingPresence()
  presenceHeartbeatTimer = setInterval(() => {
    pingPresence()
  }, presenceHeartbeatIntervalMS)
}

export default {
  onLaunch() {
    startPresenceHeartbeat()
  },
  onShow() {
    startPresenceHeartbeat()
  },
  onHide() {
    stopPresenceHeartbeat()
  }
}
</script>

<style lang="scss">
@use './styles/base.scss';
</style>
