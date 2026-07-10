import {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
  Spinner,
} from 'web'
import {
  AlertCircle,
  Download,
  FileSpreadsheet,
  FileText,
  Receipt,
  Upload,
  X,
} from 'lucide-react'

export const ImportedFiles = () => (
  <div className="flex w-80 flex-col gap-3">
    <Attachment>
      <AttachmentMedia>
        <FileSpreadsheet />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>transactions-2026-06.csv</AttachmentTitle>
        <AttachmentDescription>512 KB · 214 transactions</AttachmentDescription>
      </AttachmentContent>
      <AttachmentActions>
        <AttachmentAction aria-label="Download">
          <Download />
        </AttachmentAction>
        <AttachmentAction aria-label="Remove">
          <X />
        </AttachmentAction>
      </AttachmentActions>
    </Attachment>
    <Attachment>
      <AttachmentMedia>
        <Receipt />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>grocery-receipt-ikea-family.pdf</AttachmentTitle>
        <AttachmentDescription>128 KB · PDF</AttachmentDescription>
      </AttachmentContent>
      <AttachmentActions>
        <AttachmentAction aria-label="Remove">
          <X />
        </AttachmentAction>
      </AttachmentActions>
    </Attachment>
  </div>
)

export const UploadStates = () => (
  <div className="flex w-80 flex-col gap-3">
    <Attachment state="idle">
      <AttachmentMedia>
        <Upload />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>Bank statement (CSV)</AttachmentTitle>
        <AttachmentDescription>Drop a file or click to browse</AttachmentDescription>
      </AttachmentContent>
    </Attachment>
    <Attachment state="uploading">
      <AttachmentMedia>
        <Spinner />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>june-statement.csv</AttachmentTitle>
        <AttachmentDescription>Uploading · 42%</AttachmentDescription>
      </AttachmentContent>
      <AttachmentActions>
        <AttachmentAction aria-label="Cancel upload">
          <X />
        </AttachmentAction>
      </AttachmentActions>
    </Attachment>
    <Attachment state="processing">
      <AttachmentMedia>
        <Spinner />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>transactions-2026-05.csv</AttachmentTitle>
        <AttachmentDescription>Matching categories…</AttachmentDescription>
      </AttachmentContent>
    </Attachment>
    <Attachment state="error">
      <AttachmentMedia>
        <AlertCircle />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>bank-export.xls</AttachmentTitle>
        <AttachmentDescription>Unsupported format — use CSV</AttachmentDescription>
      </AttachmentContent>
      <AttachmentActions>
        <AttachmentAction aria-label="Remove">
          <X />
        </AttachmentAction>
      </AttachmentActions>
    </Attachment>
  </div>
)

export const Sizes = () => (
  <div className="flex w-80 flex-col gap-3">
    <Attachment>
      <AttachmentMedia>
        <FileSpreadsheet />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>transactions-2026-06.csv</AttachmentTitle>
        <AttachmentDescription>512 KB</AttachmentDescription>
      </AttachmentContent>
    </Attachment>
    <Attachment size="sm">
      <AttachmentMedia>
        <FileSpreadsheet />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>transactions-2026-06.csv</AttachmentTitle>
        <AttachmentDescription>512 KB</AttachmentDescription>
      </AttachmentContent>
    </Attachment>
    <Attachment size="xs">
      <AttachmentMedia>
        <FileSpreadsheet />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>transactions-2026-06.csv</AttachmentTitle>
      </AttachmentContent>
    </Attachment>
  </div>
)

export const ReceiptGallery = () => (
  <AttachmentGroup className="w-100">
    <Attachment orientation="vertical">
      <AttachmentMedia>
        <Receipt />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>Groceries</AttachmentTitle>
        <AttachmentDescription>−$85.20</AttachmentDescription>
      </AttachmentContent>
      <AttachmentActions>
        <AttachmentAction variant="secondary" aria-label="Remove">
          <X />
        </AttachmentAction>
      </AttachmentActions>
    </Attachment>
    <Attachment orientation="vertical">
      <AttachmentMedia>
        <FileText />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>Rent invoice</AttachmentTitle>
        <AttachmentDescription>−$1,250.00</AttachmentDescription>
      </AttachmentContent>
      <AttachmentActions>
        <AttachmentAction variant="secondary" aria-label="Remove">
          <X />
        </AttachmentAction>
      </AttachmentActions>
    </Attachment>
    <Attachment orientation="vertical">
      <AttachmentMedia>
        <Receipt />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>Transport</AttachmentTitle>
        <AttachmentDescription>−$42.50</AttachmentDescription>
      </AttachmentContent>
      <AttachmentActions>
        <AttachmentAction variant="secondary" aria-label="Remove">
          <X />
        </AttachmentAction>
      </AttachmentActions>
    </Attachment>
  </AttachmentGroup>
)
