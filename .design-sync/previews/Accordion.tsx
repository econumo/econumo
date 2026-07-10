import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from 'web'

export const SingleOpen = () => (
  <Accordion type="single" collapsible defaultValue="limits" className="w-80">
    <AccordionItem value="limits">
      <AccordionTrigger>How do budget limits work?</AccordionTrigger>
      <AccordionContent>
        Set a monthly limit per envelope — spending against Groceries or
        Transport counts toward it, and the remainder carries over.
      </AccordionContent>
    </AccordionItem>
    <AccordionItem value="sharing">
      <AccordionTrigger>Can I share an account?</AccordionTrigger>
      <AccordionContent>
        Yes — invite another user and grant admin, user, or guest access.
      </AccordionContent>
    </AccordionItem>
    <AccordionItem value="currencies">
      <AccordionTrigger>Which currencies are supported?</AccordionTrigger>
      <AccordionContent>
        Any ISO currency; rates update daily against your base currency (USD).
      </AccordionContent>
    </AccordionItem>
  </Accordion>
)

export const MultipleOpen = () => (
  <Accordion type="multiple" defaultValue={['groceries', 'transport']} className="w-80">
    <AccordionItem value="groceries">
      <AccordionTrigger>
        <span className="flex flex-1 justify-between pr-2">
          <span>Groceries</span>
          <span className="text-expense">−$385.20</span>
        </span>
      </AccordionTrigger>
      <AccordionContent>
        <div className="space-y-1">
          <div className="flex justify-between">
            <span>Whole Foods</span>
            <span className="text-expense">−$142.30</span>
          </div>
          <div className="flex justify-between">
            <span>Trader Joe's</span>
            <span className="text-expense">−$96.40</span>
          </div>
        </div>
      </AccordionContent>
    </AccordionItem>
    <AccordionItem value="transport">
      <AccordionTrigger>
        <span className="flex flex-1 justify-between pr-2">
          <span>Transport</span>
          <span className="text-expense">−$64.00</span>
        </span>
      </AccordionTrigger>
      <AccordionContent>
        <div className="flex justify-between">
          <span>Monthly transit pass</span>
          <span className="text-expense">−$64.00</span>
        </div>
      </AccordionContent>
    </AccordionItem>
    <AccordionItem value="restaurants">
      <AccordionTrigger>
        <span className="flex flex-1 justify-between pr-2">
          <span>Restaurants</span>
          <span className="text-expense">−$142.75</span>
        </span>
      </AccordionTrigger>
      <AccordionContent>
        <div className="flex justify-between">
          <span>Osteria Nova</span>
          <span className="text-expense">−$142.75</span>
        </div>
      </AccordionContent>
    </AccordionItem>
  </Accordion>
)
