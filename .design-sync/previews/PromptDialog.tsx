import { PromptDialog } from 'web'

export const CreateFolder = () => (
  <PromptDialog
    open
    onClose={() => {}}
    onSubmit={() => {}}
    title="New folder"
    inputLabel="Folder name"
    submitLabel="Create"
    cancelLabel="Cancel"
  />
)

export const RenameTag = () => (
  <PromptDialog
    open
    onClose={() => {}}
    onSubmit={() => {}}
    title="Edit tag"
    inputLabel="Tag name"
    initialValue="Vacation"
    submitLabel="Update"
    cancelLabel="Cancel"
  />
)
