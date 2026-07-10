import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from 'web'
import { Trash2 } from 'lucide-react'

export const DeleteAccountConfirm = () => (
  <AlertDialog open>
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogMedia className="bg-destructive/10 text-destructive">
          <Trash2 />
        </AlertDialogMedia>
        <AlertDialogTitle>Delete “Cash” account?</AlertDialogTitle>
        <AlertDialogDescription>
          The account and its 42 transactions will be permanently removed. This
          action cannot be undone.
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>Cancel</AlertDialogCancel>
        <AlertDialogAction variant="destructive">Delete account</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
)

export const ArchiveCategoryConfirmSmall = () => (
  <AlertDialog open>
    <AlertDialogContent size="sm">
      <AlertDialogHeader>
        <AlertDialogTitle>Archive “Restaurants”?</AlertDialogTitle>
        <AlertDialogDescription>
          Archived categories are hidden when adding new transactions but keep
          their history in reports.
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>Cancel</AlertDialogCancel>
        <AlertDialogAction>Archive</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
)
