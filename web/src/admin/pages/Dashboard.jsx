import { useQuery } from '@tanstack/react-query'
import {
  Box,
  Card,
  CardContent,
  Chip,
  Divider,
  Stack,
  Typography,
} from '@mui/material'
import CheckCircleRoundedIcon from '@mui/icons-material/CheckCircleRounded'
import CloudOffRoundedIcon from '@mui/icons-material/CloudOffRounded'
import SyncRoundedIcon from '@mui/icons-material/SyncRounded'

import { getHealth } from '../../api/health.js'
import { useRealtimeStatus } from '../../realtime/useRealtimeStatus.js'

function StatusChip({ state, onlineLabel }) {
  if (state === 'online') {
    return <Chip color="success" icon={<CheckCircleRoundedIcon />} label={onlineLabel} size="small" />
  }
  if (state === 'offline') {
    return <Chip color="error" icon={<CloudOffRoundedIcon />} label="不可用" size="small" />
  }
  return <Chip icon={<SyncRoundedIcon />} label="连接中" size="small" />
}

function StatusRow({ label, children }) {
  return (
    <Box sx={{ alignItems: 'center', display: 'flex', justifyContent: 'space-between', py: 1.5 }}>
      <Typography color="text.secondary" variant="body2">{label}</Typography>
      {children}
    </Box>
  )
}

export default function Dashboard() {
  const health = useQuery({ queryKey: ['health'], queryFn: getHealth, retry: 1 })
  const realtimeStatus = useRealtimeStatus()
  const healthState = health.isPending ? 'connecting' : health.isError ? 'offline' : 'online'

  return (
    <Stack spacing={3}>
      <Box>
        <Chip color="primary" label="Admin" size="small" sx={{ mb: 1.5 }} />
        <Typography component="h2" gutterBottom variant="h4">
          欢迎使用 Lumi 管理后台
        </Typography>
        <Typography color="text.secondary">
          管理端基础架构已经就绪，业务功能将在对应 PRD 明确后接入。
        </Typography>
      </Box>

      <Box sx={{ display: 'grid', gap: 3, gridTemplateColumns: { xs: '1fr', lg: '1fr 1fr' } }}>
        <Card variant="outlined">
          <CardContent sx={{ p: { xs: 2.5, sm: 3 }, '&:last-child': { pb: { xs: 2.5, sm: 3 } } }}>
            <Typography component="h3" gutterBottom variant="h6">服务状态</Typography>
            <StatusRow label="Go API">
              <StatusChip onlineLabel="正常" state={healthState} />
            </StatusRow>
            <Divider />
            <StatusRow label="SQLite">
              <StatusChip onlineLabel={health.data?.database || '已连接'} state={healthState} />
            </StatusRow>
          </CardContent>
        </Card>

        <Card variant="outlined">
          <CardContent sx={{ p: { xs: 2.5, sm: 3 }, '&:last-child': { pb: { xs: 2.5, sm: 3 } } }}>
            <Typography component="h3" gutterBottom variant="h6">实时通信</Typography>
            <StatusRow label="/api/v1/ws">
              <StatusChip onlineLabel="已连接" state={realtimeStatus} />
            </StatusRow>
            <Divider />
            <Typography color="text.secondary" sx={{ pt: 2 }} variant="body2">
              当前仅开放 system 基础 topic，未注册任何业务事件。
            </Typography>
          </CardContent>
        </Card>
      </Box>

      <Card variant="outlined">
        <CardContent sx={{ p: { xs: 2.5, sm: 3 }, '&:last-child': { pb: { xs: 2.5, sm: 3 } } }}>
          <Typography component="h3" gutterBottom variant="h6">控制台状态</Typography>
          <Typography color="text.secondary" variant="body2">
            当前没有可用的业务管理功能。
          </Typography>
        </CardContent>
      </Card>
    </Stack>
  )
}
