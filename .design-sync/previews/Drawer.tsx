import {
  Button,
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from 'web'

export const NewTransactionDrawer = () => (
  <Drawer open>
    <DrawerContent>
      <DrawerHeader>
        <DrawerTitle>New transaction</DrawerTitle>
        <DrawerDescription>
          Add a record to “Main account”.
        </DrawerDescription>
      </DrawerHeader>
      <DrawerFooter>
        <Button variant="destructive">Expense</Button>
        <Button className="bg-income text-white hover:bg-income/90">
          Income
        </Button>
        <Button variant="outline">Transfer</Button>
        <DrawerClose asChild>
          <Button variant="ghost">Cancel</Button>
        </DrawerClose>
      </DrawerFooter>
    </DrawerContent>
  </Drawer>
)
