import {
  NavigationMenu,
  NavigationMenuContent,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
  NavigationMenuTrigger,
  navigationMenuTriggerStyle,
} from 'web'
import { ArrowLeftRight, ChartPie, Wallet } from 'lucide-react'

export const TopNav = () => (
  <NavigationMenu viewport={false}>
    <NavigationMenuList>
      <NavigationMenuItem>
        <NavigationMenuLink href="#" className={navigationMenuTriggerStyle()}>
          Budget
        </NavigationMenuLink>
      </NavigationMenuItem>
      <NavigationMenuItem>
        <NavigationMenuTrigger>Accounts</NavigationMenuTrigger>
        <NavigationMenuContent>
          <ul className="grid w-56 gap-1">
            <li>
              <NavigationMenuLink href="#">
                <Wallet />
                Main account
              </NavigationMenuLink>
            </li>
            <li>
              <NavigationMenuLink href="#">
                <Wallet />
                Savings
              </NavigationMenuLink>
            </li>
          </ul>
        </NavigationMenuContent>
      </NavigationMenuItem>
      <NavigationMenuItem>
        <NavigationMenuTrigger>Reports</NavigationMenuTrigger>
        <NavigationMenuContent>
          <ul className="grid w-56 gap-1">
            <li>
              <NavigationMenuLink href="#">
                <ChartPie />
                Spending by category
              </NavigationMenuLink>
            </li>
          </ul>
        </NavigationMenuContent>
      </NavigationMenuItem>
      <NavigationMenuItem>
        <NavigationMenuLink href="#" className={navigationMenuTriggerStyle()}>
          Settings
        </NavigationMenuLink>
      </NavigationMenuItem>
    </NavigationMenuList>
  </NavigationMenu>
)

export const OpenAccountsMenu = () => (
  <div className="flex h-72 w-full justify-center pt-2">
    <NavigationMenu viewport={false} defaultValue="accounts" className="items-start">
      <NavigationMenuList>
        <NavigationMenuItem>
          <NavigationMenuLink href="#" className={navigationMenuTriggerStyle()}>
            Budget
          </NavigationMenuLink>
        </NavigationMenuItem>
        <NavigationMenuItem value="accounts">
          <NavigationMenuTrigger>Accounts</NavigationMenuTrigger>
          <NavigationMenuContent>
            <ul className="grid w-64 gap-1">
              <li>
                <NavigationMenuLink href="#" className="flex-col items-start gap-0.5">
                  <div className="flex items-center gap-2 font-medium">
                    <Wallet />
                    Main account
                  </div>
                  <span className="text-muted-foreground">$2,450.80 · USD</span>
                </NavigationMenuLink>
              </li>
              <li>
                <NavigationMenuLink href="#" className="flex-col items-start gap-0.5">
                  <div className="flex items-center gap-2 font-medium">
                    <Wallet />
                    Cash
                  </div>
                  <span className="text-muted-foreground">$310.00 · USD</span>
                </NavigationMenuLink>
              </li>
              <li>
                <NavigationMenuLink href="#" className="flex-col items-start gap-0.5">
                  <div className="flex items-center gap-2 font-medium">
                    <ArrowLeftRight />
                    Savings
                  </div>
                  <span className="text-muted-foreground">€11,600.00 · EUR</span>
                </NavigationMenuLink>
              </li>
            </ul>
          </NavigationMenuContent>
        </NavigationMenuItem>
        <NavigationMenuItem>
          <NavigationMenuLink href="#" className={navigationMenuTriggerStyle()}>
            Settings
          </NavigationMenuLink>
        </NavigationMenuItem>
      </NavigationMenuList>
    </NavigationMenu>
  </div>
)
