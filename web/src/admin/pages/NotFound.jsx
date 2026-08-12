import { Button, Paper, Stack, Typography } from '@mui/material'
import { Link as RouterLink } from 'react-router-dom'

export default function NotFound() {
  return (
    <Paper sx={{ px: { xs: 3, sm: 6 }, py: { xs: 6, sm: 8 }, textAlign: 'center' }} variant="outlined">
      <Stack spacing={2} sx={{ alignItems: 'center' }}>
        <Typography color="primary" fontWeight={800} variant="overline">404</Typography>
        <Typography component="h2" variant="h4">页面不存在</Typography>
        <Typography color="text.secondary">你访问的管理端页面不存在或尚未开放。</Typography>
        <Button component={RouterLink} to="/" variant="contained">返回仪表盘</Button>
      </Stack>
    </Paper>
  )
}
