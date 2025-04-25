import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api.ts';
import { ApiError } from '@/components/APIError';
import { DataTable } from '@/components/DataTable';
import { Box, GitBranch } from 'lucide-react';
import { useSearchParams } from 'react-router';

export const BranchesTable = () => {
  const [, setSearchParams] = useSearchParams();
  const { data, isLoading, error } = useQuery({
    queryKey: ['branches'],
    queryFn: () => api.getBranches(),
  });

  return (
    <div className="w-full flex-1">
      {!!error && <ApiError error={error} />}
      <DataTable
        loading={isLoading}
        columns={[
          {
            header: 'Branch name',
            accessorKey: 'branchName',
            cell: value => {
              const branchName = value.row.original?.branchName || '';
              return (
                <button
                  className="flex flex-row gap-2 items-center cursor-pointer w-full"
                  onClick={() => {
                    setSearchParams({ branch: branchName });
                  }}>
                  <GitBranch className="w-4" />
                  <span className="underline">{branchName}</span>
                </button>
              );
            },
          },
          {
            header: 'Release channel',
            size: 10,
            maxSize: 10,
            accessorKey: 'releaseChannel',
            cell: value => {
              const releaseChannel = value.row.original?.releaseChannel;
              const hasChannel = typeof releaseChannel === 'string' && releaseChannel.trim() !== '';
              
              if (!hasChannel) return <span>N/A</span>;
              
              return (
                <div className="flex flex-row gap-2 items-center">
                  <Box className="w-4" />
                  <span>{releaseChannel}</span>
                </div>
              );
            },
          },
        ]}
        data={data ?? []}
      />
    </div>
  );
};
