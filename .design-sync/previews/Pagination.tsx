import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from 'web'

export const TransactionPages = () => (
  <Pagination>
    <PaginationContent>
      <PaginationItem>
        <PaginationPrevious href="#" />
      </PaginationItem>
      <PaginationItem>
        <PaginationLink href="#">1</PaginationLink>
      </PaginationItem>
      <PaginationItem>
        <PaginationLink href="#" isActive>
          2
        </PaginationLink>
      </PaginationItem>
      <PaginationItem>
        <PaginationLink href="#">3</PaginationLink>
      </PaginationItem>
      <PaginationItem>
        <PaginationEllipsis />
      </PaginationItem>
      <PaginationItem>
        <PaginationLink href="#">12</PaginationLink>
      </PaginationItem>
      <PaginationItem>
        <PaginationNext href="#" />
      </PaginationItem>
    </PaginationContent>
  </Pagination>
)

export const FirstPage = () => (
  <Pagination>
    <PaginationContent>
      <PaginationItem>
        <PaginationPrevious
          href="#"
          aria-disabled="true"
          className="pointer-events-none opacity-50"
        />
      </PaginationItem>
      <PaginationItem>
        <PaginationLink href="#" isActive>
          1
        </PaginationLink>
      </PaginationItem>
      <PaginationItem>
        <PaginationLink href="#">2</PaginationLink>
      </PaginationItem>
      <PaginationItem>
        <PaginationLink href="#">3</PaginationLink>
      </PaginationItem>
      <PaginationItem>
        <PaginationNext href="#" />
      </PaginationItem>
    </PaginationContent>
  </Pagination>
)

export const CompactPager = () => (
  <Pagination>
    <PaginationContent>
      <PaginationItem>
        <PaginationPrevious href="#" text="Newer" />
      </PaginationItem>
      <PaginationItem>
        <span className="px-2 text-sm text-muted-foreground">Page 4 of 9</span>
      </PaginationItem>
      <PaginationItem>
        <PaginationNext href="#" text="Older" />
      </PaginationItem>
    </PaginationContent>
  </Pagination>
)
