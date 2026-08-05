import { NavLink, Outlet } from 'react-router-dom';
import type { ReactNode } from 'react';
import {
  Nav,
  NavList,
  NavItem,
  Page,
  PageSidebar,
  PageSidebarBody,
  PageGroup,
  Masthead,
  MastheadMain,
  MastheadContent,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  Button,
  NavExpandable,
} from '@patternfly/react-core';
import {
  ChartLineIcon,
  CubeIcon,
  LayerGroupIcon,
  ProcessAutomationIcon,
  ShieldAltIcon,
  GlobeIcon,
  NetworkIcon,
  ServerIcon,
} from '@patternfly/react-icons';
import { useAuth } from '../auth/AuthContext';
import { isOrgAdmin } from '../auth/orgUtils';

type NavItemDef = {
  to: string;
  label: string;
  icon: ReactNode;
  resource: string;
  verb: string;
  hideForCustomer?: boolean;
  dcAdminOnly?: boolean;
  orgAdminOrDcAdmin?: boolean;
};

const navItems: NavItemDef[] = [
  { to: '/', label: 'Overview', icon: <ChartLineIcon />, resource: 'health', verb: 'get' },
  { to: '/sites', label: 'Sites', icon: <GlobeIcon />, resource: 'sites', verb: 'list' },
  { to: '/infrastructure', label: 'Infrastructure', icon: <ServerIcon />, resource: 'infrastructure', verb: 'get' },
  { to: '/clusters', label: 'Clusters', icon: <NetworkIcon />, resource: 'clusters', verb: 'list' },
  { to: '/providers', label: 'Providers', icon: <CubeIcon />, resource: 'providers', verb: 'list', hideForCustomer: true },
  { to: '/catalog', label: 'Catalog', icon: <LayerGroupIcon />, resource: 'catalog-items', verb: 'list' },
  { to: '/instances', label: 'Catalog instances', icon: <ProcessAutomationIcon />, resource: 'catalog-item-instances', verb: 'list' },
  { to: '/policies', label: 'Policies', icon: <ShieldAltIcon />, resource: 'policies', verb: 'list', orgAdminOrDcAdmin: true },
];

export function AppLayout() {
  const { user, logout, can } = useAuth();

  const header = (
    <Masthead>
      <MastheadMain>
        <span style={{ fontWeight: 600, fontSize: '1.1rem' }}>DCM Fleet Console</span>
      </MastheadMain>
      <MastheadContent>
        <Toolbar id="masthead-tools" isFullHeight isStatic>
          <ToolbarContent>
            <ToolbarItem>
              {user?.displayName || user?.name || user?.username}
            </ToolbarItem>
            <ToolbarItem>
              <Button variant="link" onClick={() => void logout()}>
                Log out
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <span style={{ fontSize: '0.75rem', opacity: 0.75 }}>
                build {import.meta.env.VITE_BUILD_ID || 'dev'}
              </span>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );

  const sidebar = (
    <PageSidebar>
      <PageSidebarBody>
        <Nav aria-label="DCM fleet">
          <NavList>
            {navItems
              .filter((item) => {
                if (item.dcAdminOnly && !user?.dcAdmin) return false;
                if (item.orgAdminOrDcAdmin && !user?.dcAdmin && !isOrgAdmin(user)) {
                  return false;
                }
                if (item.hideForCustomer && !user?.dcAdmin && !isOrgAdmin(user)) return false;
                return can(item.resource, item.verb);
              })
              .map((item) => (
                <NavItem key={item.to}>
                  <NavLink
                    to={item.to}
                    end={item.to === '/'}
                    style={({ isActive }) => ({
                      display: 'flex',
                      gap: '0.5rem',
                      alignItems: 'center',
                      fontWeight: isActive ? 600 : 400,
                    })}
                  >
                    {item.icon}
                    {item.label}
                  </NavLink>
                </NavItem>
              ))}
            {user?.dcAdmin && (
              <NavExpandable title="Access control" isExpanded>
                <NavItem>
                  <NavLink to="/admin/orgs">Organizations</NavLink>
                </NavItem>
                <NavItem>
                  <NavLink to="/admin/users">Users</NavLink>
                </NavItem>
                <NavItem>
                  <NavLink to="/admin/groups">Groups</NavLink>
                </NavItem>
                <NavItem>
                  <NavLink to="/admin/role-bindings">Role bindings</NavLink>
                </NavItem>
                <NavItem>
                  <NavLink to="/admin/service-accounts">Service accounts</NavLink>
                </NavItem>
                <NavItem>
                  <NavLink to="/admin/audit">Audit log</NavLink>
                </NavItem>
              </NavExpandable>
            )}
            {!user?.dcAdmin && isOrgAdmin(user) && (
              <>
                <NavItem>
                  <NavLink to="/admin/members">Members</NavLink>
                </NavItem>
                <NavItem>
                  <NavLink to="/admin/llm-api-keys">LLM API keys</NavLink>
                </NavItem>
              </>
            )}
          </NavList>
        </Nav>
      </PageSidebarBody>
    </PageSidebar>
  );

  return (
    <Page header={header} sidebar={sidebar}>
      <PageGroup>
        <Outlet />
      </PageGroup>
    </Page>
  );
}
