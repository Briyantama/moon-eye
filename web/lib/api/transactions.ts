export type TransactionType = 'expense' | 'income' | 'transfer';

export interface Transaction {
  id: string;
  userId: string;
  accountId: string;
  amount: number;
  currency: string;
  type: TransactionType;
  categoryId?: string | null;
  description?: string | null;
  occurredAt: string;
  metadata?: Record<string, unknown>;
  version: number;
  lastModified: string;
  source: string;
  sheetsRowId?: string | null;
  deleted: boolean;
}

export interface ApiError {
  status: number;
  message: string;
}

const API_BASE = process.env.NEXT_PUBLIC_API_BASE ?? 'http://localhost:8080';

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let message = res.statusText;
    try {
      const data = await res.json();
      if (typeof data?.message === 'string') {
        message = data.message;
      }
    } catch {
      // ignore json parse errors
    }
    const error: ApiError = { status: res.status, message };
    throw error;
  }

  return (await res.json()) as T;
}

function authHeaders(token?: string): HeadersInit {
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
  };

  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  return headers;
}

export async function fetchTransactions(token?: string): Promise<Transaction[]> {
  const res = await fetch(`${API_BASE}/api/v1/transactions`, {
    method: 'GET',
    headers: authHeaders(token),
    credentials: 'include',
  });

  return handleResponse<Transaction[]>(res);
}

export interface CreateTransactionPayload {
  accountId: string;
  amount: number;
  currency?: string;
  type: TransactionType;
  categoryId?: string | null;
  description?: string | null;
  occurredAt: string;
  metadata?: Record<string, unknown>;
}

export async function createTransaction(
  payload: CreateTransactionPayload,
  token?: string,
): Promise<Transaction> {
  const res = await fetch(`${API_BASE}/api/v1/transactions`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify(payload),
    credentials: 'include',
  });

  return handleResponse<Transaction>(res);
}

export async function syncNow(token?: string): Promise<{ status: string }> {
  const res = await fetch(`${API_BASE}/api/v1/sync`, {
    method: 'POST',
    headers: authHeaders(token),
    credentials: 'include',
  });

  return handleResponse<{ status: string }>(res);
}

