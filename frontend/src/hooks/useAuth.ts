import { createContext, useContext } from "react";
import type { User, Organization, AppConfig } from "@/types";
import { createApi } from "@/api/client";

export interface AuthContextType {
  user: User | null;
  setUser: (user: User | null) => void;
  config: AppConfig | null;
  org: Organization | null;
  setOrg: (org: Organization | null) => void;
  orgs: Organization[];
  setOrgs: (orgs: Organization[]) => void;
  api: ReturnType<typeof createApi>;
}

const defaultApi = createApi("default");

export const AuthContext = createContext<AuthContextType>({
  user: null,
  setUser: () => {},
  config: null,
  org: null,
  setOrg: () => {},
  orgs: [],
  setOrgs: () => {},
  api: defaultApi,
});

export function useAuth() {
  return useContext(AuthContext);
}
