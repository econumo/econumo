import { ConfirmDialog } from 'web'

export const DeleteEnvelope = () => (
  <ConfirmDialog
    open
    onClose={() => {}}
    onConfirm={() => {}}
    title="Delete envelope?"
    question="Are you sure you want to delete this envelope? Its limits and history will be removed from the budget."
    confirmLabel="Delete"
    cancelLabel="Cancel"
    destructive
  />
)

export const QuestionOnly = () => (
  <ConfirmDialog
    open
    onClose={() => {}}
    onConfirm={() => {}}
    question="Unlink the Groceries category from this envelope?"
    confirmLabel="Unlink"
    cancelLabel="Cancel"
  />
)
