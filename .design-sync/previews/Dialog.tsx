import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from 'web'

export const ConfirmDialog = () => (
  <Dialog open>
    <DialogContent className="max-w-sm">
      <DialogHeader>
        <DialogTitle>Delete transaction?</DialogTitle>
        <DialogDescription>
          This will remove the $42.50 Groceries transaction from Main account.
          This action cannot be undone.
        </DialogDescription>
      </DialogHeader>
      <DialogFooter className="grid grid-cols-2 gap-3">
        <Button variant="outline">Cancel</Button>
        <Button variant="destructive">Delete</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
)
