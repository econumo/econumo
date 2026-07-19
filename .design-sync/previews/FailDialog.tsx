import { FailDialog } from 'web'

export const SignInFailed = () => (
  <FailDialog
    open
    onClose={() => {}}
    title="Sign-in failed"
    description="Check your email and password and try again. If the problem persists, contact support."
  />
)
