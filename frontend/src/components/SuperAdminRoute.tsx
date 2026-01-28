import { Navigate } from "react-router-dom";
import { useAuth } from "@/hooks/useAuth";

// Renders children only if the current user is a super-admin. Redirects
// to the dashboard otherwise. Use inside a route protected by ProtectedRoute
// (so user is guaranteed non-null at this point).
export default function SuperAdminRoute({ children }: { children: React.ReactNode }) {
  const { user } = useAuth();
  if (!user?.is_super_admin) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
}
