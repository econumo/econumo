import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarProvider,
  SidebarSeparator,
} from 'web'
import {
  ArrowLeftRight,
  Banknote,
  ChartPie,
  FolderOpen,
  PiggyBank,
  Settings,
  Wallet,
} from 'lucide-react'

/*
 * The app's real sidebar (ApplicationLayout) is hand-rolled, so these cells
 * port its content — account tree with balances, budget entry, settings
 * footer — onto the shadcn Sidebar primitives. collapsible="none" keeps the
 * sidebar statically positioned inside the card (the default offcanvas
 * variant renders position:fixed at h-svh and escapes the cell).
 */

export const AppSidebar = () => (
  <SidebarProvider className="min-h-0 w-auto">
    {/* inline height: arbitrary-value utilities (h-[460px]) are not in the compiled corpus CSS */}
    <div className="overflow-hidden rounded-lg border" style={{ height: 460 }}>
      <Sidebar collapsible="none">
        <SidebarHeader>
          <div className="flex items-center gap-2 px-2 py-1">
            <div className="grid size-8 place-items-center rounded-lg bg-econumo-yellow text-sm font-medium">
              JD
            </div>
            <div className="flex flex-col">
              <span className="text-sm font-medium">Jane Doe</span>
              <span className="text-xs text-muted-foreground">jane@example.com</span>
            </div>
          </div>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Accounts</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton isActive>
                    <Wallet />
                    <span>Main account</span>
                  </SidebarMenuButton>
                  <SidebarMenuBadge>$2,450.80</SidebarMenuBadge>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <Banknote />
                    <span>Cash</span>
                  </SidebarMenuButton>
                  <SidebarMenuBadge>$310.00</SidebarMenuBadge>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <PiggyBank />
                    <span>Savings</span>
                  </SidebarMenuButton>
                  <SidebarMenuBadge>€11,600.00</SidebarMenuBadge>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
          <SidebarSeparator />
          <SidebarGroup>
            <SidebarGroupLabel>Planning</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <ChartPie />
                    <span>Budgets</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <ArrowLeftRight />
                    <span>Transactions</span>
                  </SidebarMenuButton>
                  <SidebarMenuBadge>24</SidebarMenuBadge>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton>
                <Settings />
                <span>Settings</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </Sidebar>
    </div>
  </SidebarProvider>
)

export const FolderTreeSidebar = () => (
  <SidebarProvider className="min-h-0 w-auto">
    <div className="overflow-hidden rounded-lg border" style={{ height: 380 }}>
      <Sidebar collapsible="none">
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Folders</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <FolderOpen />
                    <span>Personal</span>
                  </SidebarMenuButton>
                  <SidebarMenuSub>
                    <SidebarMenuSubItem>
                      <SidebarMenuSubButton href="#" isActive>
                        <span>Main account</span>
                      </SidebarMenuSubButton>
                    </SidebarMenuSubItem>
                    <SidebarMenuSubItem>
                      <SidebarMenuSubButton href="#">
                        <span>Cash</span>
                      </SidebarMenuSubButton>
                    </SidebarMenuSubItem>
                  </SidebarMenuSub>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <FolderOpen />
                    <span>Family</span>
                  </SidebarMenuButton>
                  <SidebarMenuSub>
                    <SidebarMenuSubItem>
                      <SidebarMenuSubButton href="#">
                        <span>Joint savings</span>
                      </SidebarMenuSubButton>
                    </SidebarMenuSubItem>
                    <SidebarMenuSubItem>
                      <SidebarMenuSubButton href="#">
                        <span>Vacation fund</span>
                      </SidebarMenuSubButton>
                    </SidebarMenuSubItem>
                  </SidebarMenuSub>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
      </Sidebar>
    </div>
  </SidebarProvider>
)
