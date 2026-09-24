import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { UserAvatar } from '@/components/UserAvatar'
import type { BudgetCommentDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { v7 as uuidv7 } from 'uuid'
import { pluralPick } from '@/lib/plural'
import { useCreateComment, useDeleteComment, useUpdateComment } from './queries'

export interface CommentThreadProps {
  budgetId: Id
  elementId: Id
  /** first of the month, Y-m-d */
  period: string
  comments: BudgetCommentDto[]
  currentUserId: Id | undefined
  /** owner/admin may delete anyone's comment */
  canModerate: boolean
  /** archived budget or a month outside the budget's range: read the thread, write nothing */
  readOnly: boolean
  /** the fetch behind `comments` hit the 2000-item server cap and dropped the oldest */
  truncated: boolean
}

const MAX_COMMENT_RUNES = 500

function runeLength(value: string): number {
  return [...value].length
}

// Comment timestamps are the server's frozen "Y-m-d H:i:s" UTC contract, not a
// user-entered local date — parsing as local time (lib/datetime's parseDateTime)
// would skew the displayed time by the viewer's offset.
function parseServerDateTime(s: string): Date {
  const [datePart, timePart = '00:00:00'] = s.split(' ')
  const [y, m, d] = datePart.split('-').map(Number)
  const [hh, mm, ss] = timePart.split(':').map(Number)
  return new Date(Date.UTC(y, m - 1, d, hh, mm, ss))
}

// The corner triangle on a commented amount cell. The visible triangle is drawn on
// an inner span so the button carries a real hit area (a touch tap on the 6x6px
// border-only box used to land on the cell behind it) without taking any layout
// space or changing column width; the cell must be `relative`.
export function CommentMarker({ count, onOpen }: { count: number; onOpen: () => void }) {
  const { t, i18n } = useTranslation()
  return (
    <button
      type="button"
      data-testid="comment-marker"
      aria-label={pluralPick(t('budgets.page.plan.comments.marker_aria'), count, i18n.language)}
      className="absolute right-0 top-0 flex h-4 w-4 items-start justify-end"
      onClick={(e) => {
        e.stopPropagation()
        onOpen()
      }}
    >
      <span className="h-0 w-0 border-l-[6px] border-t-[6px] border-l-transparent border-t-primary" />
    </button>
  )
}

export function CommentThread({ budgetId, elementId, period, comments, currentUserId, canModerate, readOnly, truncated }: CommentThreadProps) {
  const { t, i18n } = useTranslation()
  const createComment = useCreateComment(budgetId)
  const updateComment = useUpdateComment(budgetId)
  const deleteComment = useDeleteComment(budgetId)

  const [draft, setDraft] = useState('')
  const [editingId, setEditingId] = useState<Id | null>(null)
  const [editDraft, setEditDraft] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<Id | null>(null)
  // One id per draft text: a retry after a failure the client could not tell
  // from a lost response resends the same id, so the server's idempotency
  // guard answers it instead of storing a second copy.
  const pendingPost = useRef<{ text: string; id: Id } | null>(null)
  // isPending lands a render later; two clicks in one task would both pass it
  const posting = useRef(false)

  // A budget can turn archived, or a fetched range can shrink, while this
  // popover/dialog stays mounted with an edit box or a delete confirm already
  // open from before the transition — drop both so a stale write affordance
  // never survives becoming read-only.
  useEffect(() => {
    if (readOnly) {
      setEditingId(null)
      setDeleteTarget(null)
    }
  }, [readOnly])

  // createdAt is the server's fixed-width "Y-m-d H:i:s" wire format: plain
  // ordinal comparison, not locale-aware collation, is what sorts it correctly.
  const sorted = [...comments].sort((a, b) => (a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : 0))

  function post() {
    const value = draft.trim()
    if (posting.current || !value || runeLength(value) > MAX_COMMENT_RUNES) {
      return
    }
    if (pendingPost.current?.text !== value) {
      pendingPost.current = { text: value, id: uuidv7() }
    }
    posting.current = true
    createComment.mutate(
      { id: pendingPost.current.id, elementId, period, comment: value },
      {
        onSuccess: () => {
          pendingPost.current = null
          // the composer stays editable in flight: keep a note typed meanwhile
          setDraft((current) => (current.trim() === value ? '' : current))
        },
        onSettled: () => {
          posting.current = false
        },
      },
    )
  }

  function startEdit(c: BudgetCommentDto) {
    setEditingId(c.id)
    setEditDraft(c.comment)
  }

  function saveEdit(id: Id) {
    const value = editDraft.trim()
    if (!value || runeLength(value) > MAX_COMMENT_RUNES) {
      return
    }
    updateComment.mutate({ id, comment: value }, { onSuccess: () => setEditingId(null) })
  }

  const draftRunes = runeLength(draft)
  const editRunes = runeLength(editDraft)

  return (
    <div className="flex flex-col gap-2.5">
      <p className="text-sm font-medium">{t('budgets.page.plan.comments.title')}</p>
      {truncated ? <p className="text-xs text-muted-foreground">{t('budgets.page.plan.comments.truncated')}</p> : null}
      <ul
        className="flex max-h-64 flex-col gap-3 overflow-y-auto"
        aria-label={pluralPick(t('budgets.page.plan.comments.marker_aria'), sorted.length, i18n.language)}
      >
        {sorted.length === 0 ? (
          <li className="text-sm text-muted-foreground">{t('budgets.page.plan.comments.empty')}</li>
        ) : (
          sorted.map((c) => {
            const isAuthor = currentUserId !== undefined && c.author.id === currentUserId
            const isEditing = !readOnly && editingId === c.id
            return (
              <li key={c.id} className="flex items-start gap-2">
                <UserAvatar avatar={c.author.avatar} size="xs" />
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <div className="flex flex-wrap items-baseline gap-1.5">
                    <span className="truncate text-sm font-medium">{c.author.name}</span>
                    <span className="text-xs text-muted-foreground">{parseServerDateTime(c.createdAt).toLocaleString(i18n.language)}</span>
                    {c.updatedAt !== c.createdAt ? (
                      <span className="text-xs text-muted-foreground">{t('budgets.page.plan.comments.edited')}</span>
                    ) : null}
                  </div>
                  {isEditing ? (
                    <div className="flex flex-col gap-1.5">
                      <CardField label={t('budgets.page.plan.comments.comment_label')} htmlFor={`ct-edit-${c.id}`}>
                        <Textarea
                          id={`ct-edit-${c.id}`}
                          className={`${cardFieldControlClass} resize-none`}
                          value={editDraft}
                          onChange={(e) => setEditDraft(e.target.value)}
                          autoFocus
                        />
                      </CardField>
                      <div className="flex items-center justify-between">
                        <span className="text-xs text-muted-foreground">{t('budgets.page.plan.comments.counter', { count: editRunes })}</span>
                        <div className="flex gap-2">
                          <Button type="button" variant="secondary" size="sm" onClick={() => setEditingId(null)}>
                            {t('budgets.page.plan.comments.cancel')}
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            disabled={!editDraft.trim() || editRunes > MAX_COMMENT_RUNES}
                            onClick={() => saveEdit(c.id)}
                          >
                            {t('common.button.save.label')}
                          </Button>
                        </div>
                      </div>
                    </div>
                  ) : (
                    <p className="whitespace-pre-wrap text-sm">{c.comment}</p>
                  )}
                  {!readOnly && !isEditing && (isAuthor || canModerate) ? (
                    <div className="flex gap-3">
                      {isAuthor ? (
                        <Button type="button" variant="link" size="sm" className="h-auto p-0 text-xs" onClick={() => startEdit(c)}>
                          {t('budgets.page.plan.comments.edit')}
                        </Button>
                      ) : null}
                      <Button
                        type="button"
                        variant="link"
                        size="sm"
                        className="h-auto p-0 text-xs text-destructive"
                        onClick={() => setDeleteTarget(c.id)}
                      >
                        {t('budgets.page.plan.comments.delete')}
                      </Button>
                    </div>
                  ) : null}
                </div>
              </li>
            )
          })
        )}
      </ul>
      {readOnly ? (
        <p className="text-xs text-muted-foreground">{t('budgets.page.plan.comments.read_only')}</p>
      ) : (
        <div className="flex flex-col gap-1.5">
          <CardField label={t('budgets.page.plan.comments.comment_label')} htmlFor="ct-composer">
            <Textarea
              id="ct-composer"
              className={`${cardFieldControlClass} resize-none`}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={t('budgets.page.plan.comments.composer_placeholder')}
              onKeyDown={(e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
                  e.preventDefault()
                  post()
                }
              }}
            />
          </CardField>
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">{t('budgets.page.plan.comments.counter', { count: draftRunes })}</span>
            <Button
              type="button"
              size="sm"
              disabled={createComment.isPending || !draft.trim() || draftRunes > MAX_COMMENT_RUNES}
              onClick={post}
            >
              {t('budgets.page.plan.comments.post')}
            </Button>
          </div>
        </div>
      )}
      <ConfirmDialog
        open={!readOnly && deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            deleteComment.mutate({ id: deleteTarget }, { onSettled: () => setDeleteTarget(null) })
          }
        }}
        question={t('budgets.page.plan.comments.delete_confirm')}
        confirmLabel={t('budgets.page.plan.comments.delete')}
        cancelLabel={t('budgets.page.plan.comments.cancel')}
        destructive
      />
    </div>
  )
}
