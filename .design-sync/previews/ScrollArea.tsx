import { ScrollArea, ScrollBar, Separator } from 'web'

const transactions = [
  { date: 'Jun 28', payee: 'Whole Foods', amount: '−$142.30' },
  { date: 'Jun 27', payee: 'Shell', amount: '−$52.10' },
  { date: 'Jun 26', payee: 'Osteria Nova', amount: '−$86.75' },
  { date: 'Jun 25', payee: 'Salary', amount: '+$4,200.00' },
  { date: 'Jun 24', payee: "Trader Joe's", amount: '−$96.40' },
  { date: 'Jun 23', payee: 'Metro pass', amount: '−$64.00' },
  { date: 'Jun 21', payee: 'Pharmacy', amount: '−$23.15' },
  { date: 'Jun 20', payee: 'Netflix', amount: '−$15.49' },
  { date: 'Jun 18', payee: 'Rent', amount: '−$1,450.00' },
  { date: 'Jun 17', payee: 'Coffee Lab', amount: '−$8.40' },
  { date: 'Jun 15', payee: 'Electricity', amount: '−$92.60' },
  { date: 'Jun 14', payee: 'Bookstore', amount: '−$34.90' },
]

export const TransactionList = () => (
  <ScrollArea type="always" className="w-80 rounded-md border" style={{ height: 240 }}>
    <div className="p-3">
      <h4 className="mb-2 text-sm font-medium">June transactions</h4>
      {transactions.map((tx, i) => (
        <div key={tx.date + tx.payee}>
          {i > 0 && <Separator className="my-1.5" />}
          <div className="flex items-center justify-between text-sm">
            <span className="w-14 text-muted-foreground">{tx.date}</span>
            <span className="flex-1 truncate">{tx.payee}</span>
            <span className={tx.amount.startsWith('+') ? 'text-income' : 'text-expense'}>
              {tx.amount}
            </span>
          </div>
        </div>
      ))}
    </div>
  </ScrollArea>
)

export const HorizontalAccounts = () => (
  <ScrollArea type="always" className="w-80 rounded-md border whitespace-nowrap">
    <div className="flex w-max gap-3 p-3">
      {[
        ['Main account', '$2,450.80'],
        ['Savings', '$8,300.00'],
        ['Cash', '$120.00'],
        ['Family budget', '€1,240.00'],
        ['Travel fund', '$960.00'],
      ].map(([name, balance]) => (
        <div key={name} className="w-36 shrink-0 rounded-md border p-3">
          <p className="text-sm font-medium">{name}</p>
          <p className="text-sm text-muted-foreground">{balance}</p>
        </div>
      ))}
    </div>
    <ScrollBar orientation="horizontal" />
  </ScrollArea>
)
