import { BrowserRouter } from 'react-router-dom'

import AdminRoutes from './routes.jsx'

export default function AdminApp() {
  return (
    <BrowserRouter basename="/admin">
      <AdminRoutes />
    </BrowserRouter>
  )
}
