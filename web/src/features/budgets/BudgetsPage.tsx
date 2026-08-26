import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/app/queryKeys";
import {
  Bookmark,
  ChevronDown,
  ChevronRight,
  MoreVertical,
  Plus,
} from "lucide-react";
import { v7 as uuidv7 } from "uuid";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { InfoBox } from "@/components/InfoBox";
import { ResponsiveDialog } from "@/components/ResponsiveDialog";
import { UserAvatar } from "@/components/UserAvatar";
import { UserOptions } from "@/api/dto/user";
import type { BudgetMetaDto } from "@/api/dto/budget";
import { RouterPage } from "@/app/router-pages";
import { SettingsShell } from "@/features/settings/SettingsShell";
import { AccessLevelDialog } from "@/features/connections/AccessLevelDialog";
import { ShareAccessDialog } from "@/features/connections/ShareAccessDialog";
import type { ShareEntry } from "@/features/connections/shared";
import {
  buildShareEntries,
  hasBudgetAdminAccess,
} from "@/features/connections/shared";
import { useConnections } from "@/features/connections/queries";
import {
  useUserData,
  useUpdateDefaultBudget,
  userOption,
} from "@/features/user/queries";
import {
  useArchiveBudget,
  useBudgets,
  useCreateBudget,
  useDeclineBudgetAccess,
  useDeleteBudget,
  useGrantBudgetAccess,
  useRevokeBudgetAccess,
  useUnarchiveBudget,
} from "./queries";
import { CompleteBudgetDialog } from "./CompleteBudgetDialog";
import { DuplicateBudgetDialog } from "./DuplicateBudgetDialog";
import { BudgetDialog } from "./BudgetDialog";

export function BudgetsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: user } = useUserData();
  const { data: budgets = [] } = useBudgets();
  const { data: connections = [] } = useConnections();
  const createBudget = useCreateBudget();
  const deleteBudget = useDeleteBudget();
  const updateDefaultBudget = useUpdateDefaultBudget();
  const declineAccess = useDeclineBudgetAccess();
  const grantAccess = useGrantBudgetAccess();
  const revokeAccess = useRevokeBudgetAccess();
  const archiveBudget = useArchiveBudget();
  const unarchiveBudget = useUnarchiveBudget();

  const [createOpen, setCreateOpen] = useState(false);
  const [archivedOpen, setArchivedOpen] = useState(false);
  const [duplicateTarget, setDuplicateTarget] = useState<BudgetMetaDto | null>(
    null,
  );
  const [completeTarget, setCompleteTarget] = useState<BudgetMetaDto | null>(
    null,
  );
  const [deleteTarget, setDeleteTarget] = useState<BudgetMetaDto | null>(null);
  const [declineTarget, setDeclineTarget] = useState<BudgetMetaDto | null>(
    null,
  );
  const queryClient = useQueryClient();
  const [accessBudgetId, setAccessBudgetId] = useState<string | null>(null);
  const [levelEntry, setLevelEntry] = useState<ShareEntry | null>(null);
  const [errorOpen, setErrorOpen] = useState(false);

  // read the live cache copy so grant/revoke refreshes show in the open dialog
  const accessBudget = accessBudgetId
    ? (budgets.find((b) => b.id === accessBudgetId) ?? null)
    : null;

  const defaultBudgetId = userOption(user, UserOptions.BUDGET);

  const myAccess = (budget: BudgetMetaDto) =>
    budget.access.find((a) => a.user.id === user?.id);
  const isAccepted = (budget: BudgetMetaDto) =>
    myAccess(budget)?.isAccepted === 1;

  const goTo = (budget: BudgetMetaDto) => {
    if (defaultBudgetId !== budget.id) {
      updateDefaultBudget.mutate(budget.id);
    }
    navigate(RouterPage.BUDGET);
  };

  const liveBudgets = budgets.filter((b) => b.isArchived !== 1);
  const archivedBudgets = budgets.filter((b) => b.isArchived === 1);

  const renderRow = (budget: BudgetMetaDto) => {
    const accepted = isAccepted(budget);
    const isDefault = defaultBudgetId === budget.id;
    return (
      <li
        key={budget.id}
        className="flex items-center gap-2 rounded-md px-1 py-2"
      >
        <button
          type="button"
          aria-label={
            isDefault
              ? `default budget ${budget.name}`
              : `set default ${budget.name}`
          }
          disabled={isDefault || !accepted || budget.isArchived === 1}
          className="text-muted-foreground disabled:opacity-100"
          onClick={() => updateDefaultBudget.mutate(budget.id)}
        >
          <Bookmark
            className={`size-4 ${isDefault ? "fill-current text-primary" : ""}`}
          />
        </button>
        <span className="flex min-w-0 flex-1 flex-col">
          <span
            className={`truncate text-sm ${!accepted ? "text-muted-foreground" : ""}`}
            title={budget.name}
          >
            {budget.name}
          </span>
        </span>
        {budget.access.length > 1 ? (
          <span className="flex items-center -space-x-2">
            {budget.access.map((entry) => (
              <span key={entry.user.id} title={entry.user.name}>
                <UserAvatar
                  avatar={entry.user.avatar}
                  size="sm"
                  className="size-7 ring-2 ring-background"
                />
              </span>
            ))}
          </span>
        ) : null}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`budget actions ${budget.name}`}
            >
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {accepted ? (
              <DropdownMenuItem onSelect={() => goTo(budget)}>
                {t("budgets.page.settings.list_actions.go_to")}
              </DropdownMenuItem>
            ) : null}
            {user && hasBudgetAdminAccess(budget, user.id) ? (
              <DropdownMenuItem
                onSelect={() => {
                  // grant state changes on the partner's device — refresh first
                  void queryClient.invalidateQueries({ queryKey: queryKeys.budgets });
                  setAccessBudgetId(budget.id);
                }}
              >
                {t("budgets.page.settings.list_actions.access")}
              </DropdownMenuItem>
            ) : null}
            {budget.ownerUserId === user?.id ? (
              <DropdownMenuItem onSelect={() => setDuplicateTarget(budget)}>
                {t("budgets.page.settings.list_actions.duplicate")}
              </DropdownMenuItem>
            ) : null}
            {user &&
            hasBudgetAdminAccess(budget, user.id) &&
            budget.isArchived === 0 ? (
              <DropdownMenuItem onSelect={() => setCompleteTarget(budget)}>
                {t("budgets.page.settings.list_actions.complete")}
              </DropdownMenuItem>
            ) : null}
            {user && hasBudgetAdminAccess(budget, user.id) ? (
              <DropdownMenuItem
                onSelect={() =>
                  budget.isArchived === 1
                    ? unarchiveBudget.mutate(budget.id)
                    : archiveBudget.mutate(budget.id)
                }
              >
                {budget.isArchived === 1
                  ? t("budgets.page.settings.list_actions.unarchive")
                  : t("budgets.page.settings.list_actions.archive")}
              </DropdownMenuItem>
            ) : null}
            {budget.ownerUserId !== user?.id ? (
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => setDeclineTarget(budget)}
              >
                {t("common.button.decline.label")}
              </DropdownMenuItem>
            ) : null}
            {user && hasBudgetAdminAccess(budget, user.id) ? (
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => setDeleteTarget(budget)}
              >
                {t("common.button.delete.label")}
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      </li>
    );
  };

  return (
    <SettingsShell
      title={t("budgets.page.settings.header")}
      backTo={RouterPage.SETTINGS}
      actions={
        <Button type="button" size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="size-4" />
          <span className="hidden sm:inline">
            {t("budgets.page.settings.create_budget")}
          </span>
        </Button>
      }
    >
      <InfoBox>{t("budgets.page.settings.info")}</InfoBox>
      {budgets.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">
          {t("common.list.list_empty")}
        </p>
      ) : (
        <ul className="flex flex-col">
          {liveBudgets.map((budget) => renderRow(budget))}
          {archivedBudgets.length > 0 ? (
            <li className="flex flex-col">
              <button
                type="button"
                className="flex items-center gap-2 rounded-md px-1 py-2 text-sm text-muted-foreground"
                onClick={() => setArchivedOpen((v) => !v)}
              >
                {archivedOpen ? (
                  <ChevronDown className="size-4" />
                ) : (
                  <ChevronRight className="size-4" />
                )}
                {t("budgets.page.settings.archived_group.header")} (
                {archivedBudgets.length})
              </button>
              {archivedOpen ? (
                <ul className="flex flex-col">
                  {archivedBudgets.map((budget) => renderRow(budget))}
                </ul>
              ) : null}
            </li>
          ) : null}
        </ul>
      )}

      <DuplicateBudgetDialog budget={duplicateTarget} onClose={() => setDuplicateTarget(null)} />
      <CompleteBudgetDialog budget={completeTarget} onClose={() => setCompleteTarget(null)} />

      <BudgetDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onSubmit={(form) => {
          createBudget.mutate(
            { id: uuidv7(), name: form.name, startDate: '', currencyId: form.currencyId, accountIds: form.accountIds, ownerUserId: user?.id },
            {
              onSuccess: () => setCreateOpen(false),
              onError: () => setErrorOpen(true),
            },
          );
        }}
      />

      <ShareAccessDialog
        open={accessBudget !== null && levelEntry === null}
        title={accessBudget?.name ?? ""}
        kind="budgets"
        entries={
          accessBudget && user
            ? buildShareEntries(
                connections,
                accessBudget.access,
                user.id,
                accessBudget.ownerUserId,
              )
            : []
        }
        onPick={(entry) => {
          if (entry.role !== "owner") {
            setLevelEntry(entry);
          }
        }}
        onClose={() => setAccessBudgetId(null)}
      />

      <AccessLevelDialog
        open={levelEntry !== null}
        kind="budgets"
        user={levelEntry?.user ?? null}
        role={levelEntry?.role ?? null}
        onSelect={(role) => {
          if (levelEntry && accessBudgetId) {
            grantAccess.mutate(
              { budgetId: accessBudgetId, userId: levelEntry.user.id, role },
              { onError: () => setErrorOpen(true) },
            );
          }
          setLevelEntry(null);
        }}
        onRevoke={() => {
          if (levelEntry && accessBudgetId) {
            revokeAccess.mutate({
              budgetId: accessBudgetId,
              userId: levelEntry.user.id,
            });
          }
          setLevelEntry(null);
        }}
        onClose={() => setLevelEntry(null)}
      />

      <ConfirmDialog
        open={declineTarget !== null}
        onClose={() => setDeclineTarget(null)}
        onConfirm={() => {
          if (declineTarget) {
            declineAccess.mutate(declineTarget.id, {
              onSettled: () => setDeclineTarget(null),
            });
          }
        }}
        title={t("budgets.page.settings.decline_access_modal.title")}
        question={t("budgets.page.settings.decline_access_modal.question", {
          name: declineTarget?.name ?? "",
        })}
        confirmLabel={t("common.button.decline.label")}
        cancelLabel={t("common.button.cancel.label")}
        destructive
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            deleteBudget.mutate(deleteTarget.id, {
              onSettled: () => setDeleteTarget(null),
            });
          }
        }}
        title={t("budgets.page.settings.delete_modal.title")}
        question={t("budgets.page.settings.delete_modal.question", {
          name: deleteTarget?.name ?? "",
        })}
        confirmLabel={t("common.button.delete.label")}
        cancelLabel={t("common.button.cancel.label")}
        destructive
      />

      <ResponsiveDialog
        open={errorOpen}
        onOpenChange={(o) => !o && setErrorOpen(false)}
        title={t("budgets.modal.generic_error.header")}
        description={t("budgets.modal.generic_error.description")}
      >
        <Button
          type="button"
          className="w-full h-11"
          onClick={() => setErrorOpen(false)}
        >
          {t("common.button.close.label")}
        </Button>
      </ResponsiveDialog>
    </SettingsShell>
  );
}
