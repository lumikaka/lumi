import { useState } from 'react'
import {
  AppBar,
  Box,
  Button,
  Divider,
  Drawer,
  IconButton,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Toolbar,
  Typography,
} from '@mui/material'
import ArrowBackRoundedIcon from '@mui/icons-material/ArrowBackRounded'
import DashboardRoundedIcon from '@mui/icons-material/DashboardRounded'
import MenuRoundedIcon from '@mui/icons-material/MenuRounded'
import { NavLink, Outlet } from 'react-router-dom'

const drawerWidth = 256

function Navigation({ onNavigate }) {
  return (
    <>
      <Toolbar sx={{ px: 3 }}>
        <Box
          aria-hidden="true"
          sx={{
            alignItems: 'center',
            bgcolor: 'primary.main',
            borderRadius: 2,
            color: 'primary.contrastText',
            display: 'flex',
            fontSize: 18,
            fontWeight: 800,
            height: 36,
            justifyContent: 'center',
            mr: 1.5,
            width: 36,
          }}
        >
          L
        </Box>
        <Box>
          <Typography component="p" fontWeight={700} sx={{ lineHeight: 1.2 }}>
            Lumi Admin
          </Typography>
          <Typography color="text.secondary" variant="caption">
            管理控制台
          </Typography>
        </Box>
      </Toolbar>
      <Divider />
      <Box component="nav" aria-label="管理后台导航" sx={{ p: 1.5 }}>
        <Typography
          color="text.secondary"
          sx={{ display: 'block', fontWeight: 700, px: 1.5, py: 1 }}
          variant="overline"
        >
          总览
        </Typography>
        <List disablePadding>
          <ListItemButton
            component={NavLink}
            end
            onClick={onNavigate}
            sx={{
              borderRadius: 2,
              color: 'text.secondary',
              '&:hover': { bgcolor: 'action.hover', color: 'text.primary' },
              '&.active': {
                bgcolor: 'primary.main',
                color: 'primary.contrastText',
                '& .MuiListItemIcon-root': { color: 'inherit' },
              },
              '&.active:hover': {
                bgcolor: 'primary.dark',
                color: 'primary.contrastText',
              },
            }}
            to="/"
          >
            <ListItemIcon sx={{ color: 'inherit', minWidth: 40 }}>
              <DashboardRoundedIcon />
            </ListItemIcon>
            <ListItemText primary="仪表盘" slotProps={{ primary: { sx: { fontWeight: 600 } } }} />
          </ListItemButton>
        </List>
      </Box>
    </>
  )
}

export default function AdminLayout() {
  const [mobileOpen, setMobileOpen] = useState(false)
  const drawer = <Navigation onNavigate={() => setMobileOpen(false)} />

  return (
    <Box sx={{ display: 'flex', minHeight: '100vh' }}>
      <AppBar
        color="inherit"
        elevation={0}
        position="fixed"
        sx={{
          borderBottom: 1,
          borderColor: 'divider',
          ml: { md: `${drawerWidth}px` },
          width: { md: `calc(100% - ${drawerWidth}px)` },
        }}
      >
        <Toolbar>
          <IconButton
            aria-label="打开导航"
            color="inherit"
            edge="start"
            onClick={() => setMobileOpen(true)}
            sx={{ display: { md: 'none' }, mr: 2 }}
          >
            <MenuRoundedIcon />
          </IconButton>
          <Typography component="h1" sx={{ flexGrow: 1 }} variant="h6">
            管理后台
          </Typography>
          <Button color="inherit" href="/" startIcon={<ArrowBackRoundedIcon />}>
            返回用户端
          </Button>
        </Toolbar>
      </AppBar>

      <Box component="aside" sx={{ flexShrink: { md: 0 }, width: { md: drawerWidth } }}>
        <Drawer
          ModalProps={{ keepMounted: true }}
          onClose={() => setMobileOpen(false)}
          open={mobileOpen}
          sx={{
            display: { xs: 'block', md: 'none' },
            '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
          }}
          variant="temporary"
        >
          {drawer}
        </Drawer>
        <Drawer
          open
          sx={{
            display: { xs: 'none', md: 'block' },
            '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
          }}
          variant="permanent"
        >
          {drawer}
        </Drawer>
      </Box>

      <Box component="main" sx={{ flexGrow: 1, minWidth: 0, p: { xs: 2, sm: 3, lg: 4 } }}>
        <Toolbar />
        <Box sx={{ maxWidth: 1200, mx: 'auto' }}>
          <Outlet />
        </Box>
      </Box>
    </Box>
  )
}
