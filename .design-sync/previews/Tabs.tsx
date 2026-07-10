import { Tabs, TabsContent, TabsList, TabsTrigger } from 'web'

const rows = (
  <div className="w-72 space-y-1 rounded-lg border p-3">
    <div className="flex justify-between text-sm">
      <span>Groceries</span>
      <span className="text-expense">−$385.20</span>
    </div>
    <div className="flex justify-between text-sm">
      <span>Restaurants</span>
      <span className="text-expense">−$142.75</span>
    </div>
    <div className="flex justify-between text-sm">
      <span>Transport</span>
      <span className="text-expense">−$56.40</span>
    </div>
  </div>
)

export const TransactionTabs = () => (
  <Tabs defaultValue="expenses">
    <TabsList>
      <TabsTrigger value="expenses">Expenses</TabsTrigger>
      <TabsTrigger value="income">Income</TabsTrigger>
      <TabsTrigger value="transfers">Transfers</TabsTrigger>
    </TabsList>
    <TabsContent value="expenses">{rows}</TabsContent>
    <TabsContent value="income">
      <div className="w-72 space-y-1 rounded-lg border p-3">
        <div className="flex justify-between text-sm">
          <span>Salary</span>
          <span className="text-income">+$4,200.00</span>
        </div>
      </div>
    </TabsContent>
  </Tabs>
)

export const LineTabs = () => (
  <Tabs defaultValue="month">
    <TabsList variant="line">
      <TabsTrigger value="week">Week</TabsTrigger>
      <TabsTrigger value="month">Month</TabsTrigger>
      <TabsTrigger value="year">Year</TabsTrigger>
    </TabsList>
    <TabsContent value="month">
      <p className="w-72 pt-1 text-sm text-muted-foreground">
        Spent <span className="text-expense">$1,284.35</span> of $1,800.00
        budgeted in June.
      </p>
    </TabsContent>
  </Tabs>
)

export const WithDisabledTab = () => (
  <Tabs defaultValue="accounts">
    <TabsList>
      <TabsTrigger value="accounts">Accounts</TabsTrigger>
      <TabsTrigger value="shared">Shared</TabsTrigger>
      <TabsTrigger value="archived" disabled>
        Archived
      </TabsTrigger>
    </TabsList>
    <TabsContent value="accounts">
      <div className="w-72 space-y-1 rounded-lg border p-3 text-sm">
        <div className="flex justify-between">
          <span>Main account</span>
          <span>$2,450.80</span>
        </div>
        <div className="flex justify-between">
          <span>Cash</span>
          <span>$310.00</span>
        </div>
      </div>
    </TabsContent>
  </Tabs>
)

export const VerticalTabs = () => (
  <Tabs defaultValue="profile" orientation="vertical" className="w-80">
    <TabsList>
      <TabsTrigger value="profile">Profile</TabsTrigger>
      <TabsTrigger value="currencies">Currencies</TabsTrigger>
      <TabsTrigger value="connections">Connections</TabsTrigger>
    </TabsList>
    <TabsContent value="profile">
      <p className="pt-1 text-sm text-muted-foreground">
        Base currency USD · Timezone Europe/Amsterdam
      </p>
    </TabsContent>
  </Tabs>
)
