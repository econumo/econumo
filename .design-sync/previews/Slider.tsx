import { Label, Slider } from 'web'

export const Default = () => (
  <div className="flex w-80 flex-col gap-3">
    <div className="flex items-baseline justify-between">
      <Label htmlFor="slider-alert" className="font-normal">
        Budget alert threshold
      </Label>
      <span className="text-sm text-muted-foreground">75%</span>
    </div>
    <Slider id="slider-alert" defaultValue={[75]} min={0} max={100} step={5} />
  </div>
)

export const Range = () => (
  <div className="flex w-80 flex-col gap-3">
    <div className="flex items-baseline justify-between">
      <Label htmlFor="slider-range" className="font-normal">
        Amount filter
      </Label>
      <span className="text-sm text-muted-foreground">$50 – $400</span>
    </div>
    <Slider id="slider-range" defaultValue={[50, 400]} min={0} max={500} step={10} />
  </div>
)

export const Disabled = () => (
  <div className="flex w-80 flex-col gap-3">
    <div className="flex items-baseline justify-between">
      <Label htmlFor="slider-disabled" className="font-normal text-muted-foreground">
        Savings goal progress
      </Label>
      <span className="text-sm text-muted-foreground">40%</span>
    </div>
    <Slider id="slider-disabled" disabled defaultValue={[40]} min={0} max={100} />
  </div>
)
