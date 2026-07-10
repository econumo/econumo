import {
  Button,
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from 'web'

export const AccountCard = () => (
  <Card className="w-80">
    <CardHeader>
      <CardTitle>Main account</CardTitle>
      <CardDescription>Personal · USD</CardDescription>
      <CardAction>
        <Button variant="ghost" size="sm">
          Edit
        </Button>
      </CardAction>
    </CardHeader>
    <CardContent>
      <p className="text-2xl font-medium">$2,450.80</p>
      <p className="text-sm text-muted-foreground">Balance as of today</p>
    </CardContent>
    <CardFooter>
      <Button size="sm">Add transaction</Button>
    </CardFooter>
  </Card>
)

export const SummaryCard = () => (
  <Card className="w-80">
    <CardHeader>
      <CardTitle>June budget</CardTitle>
      <CardDescription>12 of 30 envelopes over limit</CardDescription>
    </CardHeader>
    <CardContent className="space-y-1">
      <div className="flex justify-between text-sm">
        <span>Groceries</span>
        <span className="text-expense">−$385.20</span>
      </div>
      <div className="flex justify-between text-sm">
        <span>Salary</span>
        <span className="text-income">+$4,200.00</span>
      </div>
      <div className="flex justify-between text-sm">
        <span>Restaurants</span>
        <span className="text-expense">−$142.75</span>
      </div>
    </CardContent>
  </Card>
)
