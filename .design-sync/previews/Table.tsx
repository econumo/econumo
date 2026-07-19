import {
  Badge,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from 'web'

const transactions = [
  {
    date: 'Jun 28, 2026',
    category: 'Groceries',
    account: 'Main account',
    amount: '−$85.20',
    type: 'expense',
  },
  {
    date: 'Jun 27, 2026',
    category: 'Salary',
    account: 'Main account',
    amount: '+$4,200.00',
    type: 'income',
  },
  {
    date: 'Jun 26, 2026',
    category: 'Restaurants',
    account: 'Cash',
    amount: '−$42.50',
    type: 'expense',
  },
  {
    date: 'Jun 25, 2026',
    category: 'Transport',
    account: 'Cash',
    amount: '−$18.00',
    type: 'expense',
  },
]

export const TransactionsTable = () => (
  <div style={{ width: 520 }}>
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Date</TableHead>
          <TableHead>Category</TableHead>
          <TableHead>Account</TableHead>
          <TableHead className="text-right">Amount</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {transactions.map((tx) => (
          <TableRow key={tx.date}>
            <TableCell className="text-muted-foreground">{tx.date}</TableCell>
            <TableCell className="font-medium">{tx.category}</TableCell>
            <TableCell>{tx.account}</TableCell>
            <TableCell
              className={
                tx.type === 'income'
                  ? 'text-right text-income'
                  : 'text-right text-expense'
              }
            >
              {tx.amount}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
      <TableFooter>
        <TableRow>
          <TableCell colSpan={3}>Net change · June</TableCell>
          <TableCell className="text-right text-income">+$4,054.30</TableCell>
        </TableRow>
      </TableFooter>
    </Table>
  </div>
)

export const AccountBalancesTable = () => (
  <div style={{ width: 420 }}>
    <Table>
      <TableCaption>Balances as of end of today (USD)</TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead>Account</TableHead>
          <TableHead>Currency</TableHead>
          <TableHead className="text-right">Balance</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow>
          <TableCell className="font-medium">Main account</TableCell>
          <TableCell>USD</TableCell>
          <TableCell className="text-right">$2,450.80</TableCell>
        </TableRow>
        <TableRow>
          <TableCell className="font-medium">Savings</TableCell>
          <TableCell>EUR</TableCell>
          <TableCell className="text-right">€8,120.00</TableCell>
        </TableRow>
        <TableRow data-state="selected">
          <TableCell className="font-medium">
            <span className="flex items-center gap-2">
              Cash <Badge variant="secondary">selected</Badge>
            </span>
          </TableCell>
          <TableCell>USD</TableCell>
          <TableCell className="text-right">$132.45</TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </div>
)
