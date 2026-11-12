import { useRef } from 'react'
import type { ReactNode } from 'react'
import { Plus } from 'lucide-react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import type { DebtRuleDto, DebtRuleOptionsDto } from '@/api/dto/debt'
import type { Id } from '@/api/types'
import { useUserData } from '@/features/user/queries'
import { PaymentsFromEditor } from './PaymentsFromEditor'
import type { Option, OptionGroup, PaymentsFromChoices } from './PaymentsFromEditor'
import { emptyRule, MAX_RULES, ruleDraft, ruleOwnerLabel } from './rules'
import type { PaymentsFrom, RuleDraft } from './rules'

export interface RulesEditorProps {
  value: RuleDraft[]
  onChange: (next: RuleDraft[]) => void
  /** the budget's participants and what a rule may name; undefined while loading */
  options: DebtRuleOptionsDto | undefined
  /** the debt's stored rules: read-only they are the sentences; editing, their
      names stand in for picks the options no longer list (an archived
      category, a former participant's) */
  initialRules?: DebtRuleDto[]
  /** converting a category row: the first rule keeps that category */
  lockedCategoryId?: Id
  error?: string
  readOnly?: boolean
}

const noop = () => {}

const categoryOption = (o: { id: Id; name: string; icon?: string }): Option => ({ id: o.id, name: o.name, icon: o.icon || 'pending' })

/** the stored rule's own names as the only choices: its sentence read aloud */
function storedChoices(rule: DebtRuleDto, viewerId: Id | undefined, t: TFunction): PaymentsFromChoices {
  return {
    categories: [{ options: [categoryOption({ id: rule.categoryId, name: ruleOwnerLabel(rule, viewerId, t), icon: rule.categoryIcon })] }],
    payees: rule.payeeId ? [{ id: rule.payeeId, name: rule.payeeName }] : [],
    tags: rule.tagId ? [{ id: rule.tagId, name: rule.tagName }] : [],
    labels: rule.labelId ? [{ id: rule.labelId, name: rule.labelName }] : [],
  }
}

function RulesCard({ children, footer }: { children: ReactNode; footer?: ReactNode }) {
  const { t } = useTranslation()
  return (
    <div role="group" aria-label={t('debts.rules.heading')} className="flex flex-col gap-1.5 rounded-lg bg-econumo-card px-4 py-2.5">
      <span className="text-[11px] text-muted-foreground">{t('debts.rules.heading')}</span>
      <div className="flex flex-col divide-y divide-border [&>*]:py-1.5 [&>*:first-child]:pt-0 [&>*:last-child]:pb-0">{children}</div>
      {footer}
    </div>
  )
}

/** The debt's stored rules read out, a partner's category with its owner's name */
export function RulesReadOnly({ rules }: { rules: DebtRuleDto[] }) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  return (
    <RulesCard>
      {rules.map((rule) => (
        <PaymentsFromEditor key={rule.id} value={ruleDraft(rule)} onChange={noop} choices={storedChoices(rule, user?.id, t)} readOnly />
      ))}
    </RulesCard>
  )
}

/**
 * Which payments count as the debt's: one sentence per rule (1..10), each on
 * a category of any participant of the budget. The category picker groups
 * the categories by participant, the caller's first; the payee and tag
 * pickers list what the chosen category's owner has. Read-only, the stored
 * rules are read out, a partner's category with its owner's name.
 */
export function RulesEditor({ value, onChange, options, initialRules = [], lockedCategoryId, error, readOnly = false }: RulesEditorProps) {
  const { t } = useTranslation()
  // a line's React key follows its draft through edits (each edit is a new
  // object), so removing a line never hands its picker to the next one
  const lineKeys = useRef(new WeakMap<RuleDraft, number>())
  const nextKey = useRef(0)
  const keyOf = (rule: RuleDraft) => {
    let key = lineKeys.current.get(rule)
    if (key === undefined) {
      key = nextKey.current++
      lineKeys.current.set(rule, key)
    }
    return key
  }

  if (readOnly) {
    return <RulesReadOnly rules={initialRules} />
  }

  const participants = options?.participants ?? []
  const stored = (pick: (rule: DebtRuleDto) => boolean) => initialRules.find(pick)
  // with no category chosen yet, the caller's (listed first) stand in
  const ownerOf = (categoryId: Id | null): Id | null =>
    (categoryId === null
      ? null
      : (participants.find((p) => p.categories.some((c) => c.id === categoryId))?.userId ?? stored((r) => r.categoryId === categoryId)?.ownerUserId)) ??
    participants[0]?.userId ??
    null

  const choicesFor = (rule: PaymentsFrom): PaymentsFromChoices => {
    const groups: (OptionGroup & { userId: Id })[] = participants.map((p) => ({ userId: p.userId, heading: p.name, options: p.categories.map(categoryOption) }))
    const chosen = rule.categoryId
    if (chosen && !groups.some((g) => g.options.some((o) => o.id === chosen))) {
      const known = stored((r) => r.categoryId === chosen)
      if (known) {
        const option = categoryOption({ id: chosen, name: known.categoryName, icon: known.categoryIcon })
        const group = groups.find((g) => g.userId === known.ownerUserId)
        if (group) {
          group.options = [...group.options, option]
        } else {
          groups.push({ userId: known.ownerUserId, heading: known.ownerName, options: [option] })
        }
      }
    }
    const owner = participants.find((p) => p.userId === ownerOf(chosen))
    // a pick the owner's lists no longer hold keeps its stored name
    const withChosen = (list: Option[], id: Id | null, name: (r: DebtRuleDto) => string, idOf: (r: DebtRuleDto) => Id | null): Option[] => {
      if (!id || list.some((o) => o.id === id)) {
        return list
      }
      const known = stored((r) => idOf(r) === id)
      return known ? [...list, { id, name: name(known) }] : list
    }
    return {
      categories: groups.length > 1 ? groups : groups.map((g) => ({ options: g.options })),
      payees: withChosen(owner?.payees ?? [], rule.payeeId, (r) => r.payeeName, (r) => r.payeeId),
      tags: withChosen(owner?.tags ?? [], rule.tagId, (r) => r.tagName, (r) => r.tagId),
      labels: withChosen(owner?.labels ?? [], rule.labelId, (r) => r.labelName, (r) => r.labelId),
    }
  }

  const lockedIndex = lockedCategoryId && value[0]?.categoryId === lockedCategoryId ? 0 : -1
  const change = (index: number, next: RuleDraft) => {
    const before = value[index]
    // another owner's category: the previous owner's payee and tags narrow nothing there
    const narrowed =
      next.categoryId !== before.categoryId && ownerOf(next.categoryId) !== ownerOf(before.categoryId) ? { ...next, payeeId: null, tagId: null, labelId: null } : next
    lineKeys.current.set(narrowed, keyOf(before))
    onChange(value.map((rule, i) => (i === index ? narrowed : rule)))
  }

  return (
    <div className="flex flex-col gap-1">
      <RulesCard
        footer={
          value.length < MAX_RULES ? (
            <button type="button" className="inline-flex items-center gap-1 self-start text-sm text-econumo-link hover:underline" onClick={() => onChange([...value, emptyRule()])}>
              <Plus className="size-4" />
              {t('debts.rules.add_category')}
            </button>
          ) : null
        }
      >
        {value.map((rule, index) => (
          <PaymentsFromEditor
            key={keyOf(rule)}
            value={rule}
            onChange={(next) => change(index, next)}
            choices={choicesFor(rule)}
            categoryLocked={index === lockedIndex}
            onRemove={value.length > 1 && index !== lockedIndex ? () => onChange(value.filter((_, i) => i !== index)) : undefined}
          />
        ))}
      </RulesCard>
      {error ? <p className="px-1 text-sm text-destructive">{error}</p> : null}
    </div>
  )
}
