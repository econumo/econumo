import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from 'web'

const accounts = [
  { name: 'Main account', currency: 'USD', balance: '$2,450.80' },
  { name: 'Savings', currency: 'EUR', balance: '€8,120.00' },
  { name: 'Cash', currency: 'USD', balance: '$132.45' },
]

export const AccountsCarousel = () => (
  <div className="px-12 py-2">
    <Carousel className="w-64">
      <CarouselContent>
        {accounts.map((account) => (
          <CarouselItem key={account.name}>
            <Card>
              <CardHeader>
                <CardTitle>{account.name}</CardTitle>
                <CardDescription>{account.currency}</CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-2xl font-medium">{account.balance}</p>
                <p className="text-sm text-muted-foreground">
                  Balance as of today
                </p>
              </CardContent>
            </Card>
          </CarouselItem>
        ))}
      </CarouselContent>
      <CarouselPrevious />
      <CarouselNext />
    </Carousel>
  </div>
)

const months = [
  { month: 'April', spent: '$1,490.00' },
  { month: 'May', spent: '$1,975.00' },
  { month: 'June', spent: '$1,730.00' },
  { month: 'July', spent: '$612.40' },
]

export const MonthPeekCarousel = () => (
  <div className="px-12 py-2">
    <Carousel style={{ width: 420 }} opts={{ align: 'start' }}>
      <CarouselContent>
        {months.map((m) => (
          <CarouselItem key={m.month} style={{ flexBasis: '50%' }}>
            <Card>
              <CardHeader>
                <CardTitle>{m.month}</CardTitle>
                <CardDescription>Spending</CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-xl font-medium text-expense">−{m.spent}</p>
              </CardContent>
            </Card>
          </CarouselItem>
        ))}
      </CarouselContent>
      <CarouselPrevious />
      <CarouselNext />
    </Carousel>
  </div>
)
