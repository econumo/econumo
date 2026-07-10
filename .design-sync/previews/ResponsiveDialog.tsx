import { Button, Input, Label, ResponsiveDialog, dialogActionsClass } from 'web'

export const InviteCode = () => (
  <ResponsiveDialog open onOpenChange={() => {}} title="Invite code">
    <div className="flex flex-col gap-2">
      <p className="text-sm text-muted-foreground">
        Share this code with the person you want to connect accounts with. It expires in 30 minutes.
      </p>
      <p className="py-3 text-center font-mono text-3xl tracking-widest">XK7QM2</p>
      <Button type="button" className="w-full h-11">
        OK
      </Button>
    </div>
  </ResponsiveDialog>
)

export const CapsHeaderWithFooter = () => (
  <ResponsiveDialog
    open
    onOpenChange={() => {}}
    title="Rename account"
    description="Savings · USD"
    caps
    footer={
      <div className={dialogActionsClass}>
        <Button type="button" variant="secondary">
          Cancel
        </Button>
        <Button type="button">Save</Button>
      </div>
    }
  >
    <div className="flex flex-col gap-2">
      <Label htmlFor="rd-account-name">Account name</Label>
      <Input id="rd-account-name" defaultValue="Savings" />
    </div>
  </ResponsiveDialog>
)
