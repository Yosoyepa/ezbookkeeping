import type { NameAndDisplayName } from '@/core/base.ts';

export const ALL_MCP_TOOLS: NameAndDisplayName[] = [
    {
        name: 'query_all_accounts',
        displayName:'Query All Accounts'
    },
    {
        name: 'query_all_accounts_balance',
        displayName:'Query All Account Balances'
    },
    {
        name: 'query_all_transaction_categories',
        displayName:'Query All Transaction Categories'
    },
    {
        name: 'query_all_transaction_tags',
        displayName:'Query All Transaction Tags'
    },
    {
        name: 'query_latest_exchange_rates',
        displayName:'Query Latest Exchange Rates'
    },
    {
        name: 'query_transactions',
        displayName:'Query Transactions'
    },
    {
        name: 'add_transaction',
        displayName:'Add Transaction'
    },
    {
        name: 'get_receipt_image',
        displayName:'Get Receipt Image'
    },
    {
        name: 'list_inbox_items',
        displayName:'List Inbox Items'
    },
    {
        name: 'get_inbox_item',
        displayName:'Get Inbox Item'
    },
    {
        name: 'resolve_inbox_item',
        displayName:'Resolve Inbox Item'
    },
    {
        name: 'discard_inbox_item',
        displayName:'Discard Inbox Item'
    },
    {
        name: 'add_transactions_batch',
        displayName:'Add Transactions In Batch'
    },
    {
        name: 'update_transaction',
        displayName:'Update Transaction'
    },
    {
        name: 'get_spending_summary',
        displayName:'Get Spending Summary'
    }
];

export const ALL_MCP_TOOL_NAMES: string[] = ALL_MCP_TOOLS.map(tool => tool.name);

export const DEFAULT_ENABLED_MCP_TOOL_NAMES: string[] = [
    'query_all_accounts',
    'query_all_accounts_balance',
    'query_all_transaction_categories',
    'query_all_transaction_tags',
    'query_latest_exchange_rates',
    'query_transactions',
    'add_transaction',
    'get_receipt_image',
    'list_inbox_items',
    'get_inbox_item',
    'resolve_inbox_item',
    'discard_inbox_item',
    'add_transactions_batch',
    'update_transaction',
    'get_spending_summary'
]
