import { AvatarPickerDialog } from 'web'

// The current user comes from useUserData(), served by the bundle's
// EconumoPreviewProvider (seeded "Anna Kovaleva", avatar "owl:teal") — the
// dialog opens seeded from that saved value.
export const PickAvatar = () => <AvatarPickerDialog open onClose={() => {}} />
