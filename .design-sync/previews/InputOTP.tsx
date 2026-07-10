import { InputOTP, InputOTPGroup, InputOTPSeparator, InputOTPSlot } from 'web'

export const ConfirmationCode = () => (
  <div className="flex flex-col items-start gap-2">
    <span className="text-sm text-muted-foreground">Enter the 6-digit code we emailed you</span>
    <InputOTP maxLength={6} value="428019" onChange={() => {}}>
      <InputOTPGroup>
        <InputOTPSlot index={0} />
        <InputOTPSlot index={1} />
        <InputOTPSlot index={2} />
      </InputOTPGroup>
      <InputOTPSeparator />
      <InputOTPGroup>
        <InputOTPSlot index={3} />
        <InputOTPSlot index={4} />
        <InputOTPSlot index={5} />
      </InputOTPGroup>
    </InputOTP>
  </div>
)

export const Empty = () => (
  <InputOTP maxLength={6} value="" onChange={() => {}}>
    <InputOTPGroup>
      <InputOTPSlot index={0} />
      <InputOTPSlot index={1} />
      <InputOTPSlot index={2} />
    </InputOTPGroup>
    <InputOTPSeparator />
    <InputOTPGroup>
      <InputOTPSlot index={3} />
      <InputOTPSlot index={4} />
      <InputOTPSlot index={5} />
    </InputOTPGroup>
  </InputOTP>
)

export const FourDigitPin = () => (
  <InputOTP maxLength={4} value="20" onChange={() => {}}>
    <InputOTPGroup>
      <InputOTPSlot index={0} />
      <InputOTPSlot index={1} />
      <InputOTPSlot index={2} />
      <InputOTPSlot index={3} />
    </InputOTPGroup>
  </InputOTP>
)

export const Disabled = () => (
  <InputOTP maxLength={6} value="428019" onChange={() => {}} disabled>
    <InputOTPGroup>
      <InputOTPSlot index={0} />
      <InputOTPSlot index={1} />
      <InputOTPSlot index={2} />
    </InputOTPGroup>
    <InputOTPSeparator />
    <InputOTPGroup>
      <InputOTPSlot index={3} />
      <InputOTPSlot index={4} />
      <InputOTPSlot index={5} />
    </InputOTPGroup>
  </InputOTP>
)
