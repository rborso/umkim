import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from './AuthContext';

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();
  const location = useLocation();

  if (loading) {
    return <div style={{ padding: '2rem' }}>Loading session…</div>;
  }
  if (!user) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }
  return <>{children}</>;
}
