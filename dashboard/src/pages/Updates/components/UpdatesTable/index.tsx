import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api.ts';
import { ApiError } from '@/components/APIError';
import { DataTable } from '@/components/DataTable';
import { GitBranch, Milestone, Rss } from 'lucide-react';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Badge } from '@/components/ui/badge.tsx';

export const UpdatesTable = ({
  branch,
  runtimeVersion,
}: {
  branch: string;
  runtimeVersion: string;
}) => {
  const { data, isLoading, error } = useQuery({
    queryKey: ['updates'],
    queryFn: () => api.getUpdates(branch, runtimeVersion),
  });

  return (
    <div className="w-full flex-1">
      <Breadcrumb className="mb-2">
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink href="/dashboard" className="flex items-center gap-2 underline">
              <GitBranch className="w-4" />
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{branch}</BreadcrumbPage>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbLink
              href={`/dashboard?branch=${branch}`}
              className="flex items-center gap-2 underline">
              <Milestone className="w-4" />
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{runtimeVersion}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
      {!!error && <ApiError error={error} />}
      <DataTable
        loading={isLoading}
        columns={[
          {
            header: 'ID',
            accessorKey: 'updateId',
            cell: value => {
              const updateId = value.row.original?.updateId || 'Unknown';
              return (
                <span className="flex flex-row gap-2 items-center w-full">
                  <Rss className="w-4" />
                  {updateId}
                </span>
              );
            },
          },
          {
            header: 'UUID',
            accessorKey: 'updateUUID',
            cell: value => {
              return value.row.original?.updateUUID || 'N/A';
            },
          },
          {
            header: 'Platform',
            accessorKey: 'platform',
            cell: value => {
              const platform = value.row.original?.platform || '';
              const isIos = platform === 'ios';
              const isAndroid = platform === 'android';
              return (
                <div className="flex flex-row items-center gap-2">
                  {isIos && <img src="@/assets/apple.svg" className="w-4" alt="apple" />}
                  {isAndroid && <img src="@/assets/android.svg" className="w-4" alt="android" />}
                  {!isIos && !isAndroid && <span>Unknown</span>}
                </div>
              );
            },
          },
          {
            header: 'Commit',
            accessorKey: 'commitHash',
            cell: value => {
              const commitHash = value.row.original?.commitHash;
              if (!commitHash) {
                return <Badge variant="secondary" className="text-xs">N/A</Badge>;
              }
              return (
                <Badge variant="secondary" className="text-xs">
                  {typeof commitHash === 'string' ? commitHash.slice(0, 7) : commitHash}
                </Badge>
              );
            },
          },
          {
            header: 'Published at',
            accessorKey: 'createdAt',
            cell: ({ row }) => {
              const createdAt = row.original?.createdAt;
              if (!createdAt) {
                return <Badge variant="outline">Unknown</Badge>;
              }
              
              try {
                const date = new Date(createdAt);
                if (isNaN(date.getTime())) {
                  return <Badge variant="outline">Invalid date</Badge>;
                }
                
                return (
                  <Badge variant="outline">
                    {date.toLocaleDateString('en-GB', {
                      year: 'numeric',
                      month: 'long',
                      day: 'numeric',
                      hour: 'numeric',
                      minute: 'numeric',
                      second: 'numeric',
                    })}
                  </Badge>
                );
              } catch {
                return <Badge variant="outline">Invalid date</Badge>;
              }
            },
          },
        ]}
        data={data ?? []}
      />
    </div>
  );
};
