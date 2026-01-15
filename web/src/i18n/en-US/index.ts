export default {
  // blocks
  'blocks': {
    'help': {
      'label': '© econumo.com',
      'url': 'https://econumo.com/'
    },
    'website': {
      'label': 'Econumo',
    },
    'list': {
      'list_empty': 'List is empty',
      'order_list': 'Reorder list'
    },
    'main': {
      'budget': 'Budget',
      'onboarding': 'Onboarding',
      'create_account': 'Add an account'
    },
  },
  // elements
  'elements': {
    'econumo': { 'label': 'Econumo' },
    'folders': { 'default_folder': 'All accounts' },
    'account': { 'name_hidden': '[Hidden Account]' },
    'switch_to_account': 'Switch to',
    'form': {
      'account': {
        'folder': {
          'label': 'Name',
          'validation': {
            'empty_name': 'Enter folder name',
            'error_name_length': 'The folder name must be between 3 and 64 characters'
          }
        },
        'name': {
          'label': 'Name',
          'placeholder': 'Enter name',
          'validation': {
            'required_field': 'Required field',
            'invalid_name': 'The account name must be between 3 and 64 characters'
          }
        },
        'balance': {
          'label': 'Balance',
          'placeholder': 'Enter balance',
          'validation': {
            'required_field': 'Required field',
            'invalid_number': 'Enter a valid number'
          }
        },
        'currency': { 'label': 'Currency' }
      },
    },
    'button': {
      'ok': { 'label': 'Ok' },
      'close': { 'label': 'Close' },
      'cancel': { 'label': 'Cancel' },
      'create': { 'label': 'Create' },
      'add': { 'label': 'Add' },
      'edit': { 'label': 'Edit' },
      'set_as_default': { 'label': 'Set as default' },
      'update': { 'label': 'Update' },
      'save': { 'label': 'Save' },
      'replace': { 'label': 'Replace' },
      'delete': { 'label': 'Delete' },
      'reset': { 'label': 'Reset' },
      'copy': { 'label': 'Copy' },
      'view': { 'label': 'View' },
      'up': { 'label': 'Move up' },
      'down': { 'label': 'Move down' },
      'hide': { 'label': 'Hide' },
      'show': { 'label': 'Show' },
      'accept': { 'label': 'Accept' },
      'transfer': { 'label': 'Transfer' },
      'expand': { 'label': 'Expand' },
      'collapse': { 'label': 'Collapse' },
      'decline': { 'label': 'Decline' },
      'back': { 'label': 'Back' },
      'next': { 'label': 'Next' },
      'import': { 'label': 'Import' },
      'export': { 'label': 'Export' },
      'change': { 'label': 'Change' }
    },
    'date': {
      'month_short': {
        'jan': 'Jan',
        'feb': 'Feb',
        'mar': 'Mar',
        'apr': 'Apr',
        'may': 'May',
        'jun': 'Jun',
        'jul': 'Jul',
        'aug': 'Aug',
        'sep': 'Sep',
        'oct': 'Oct',
        'nov': 'Nov',
        'dec': 'Dec'
      }
    },
    'validation': {
      'required_field': 'Required field',
      'invalid_number': 'Enter a valid number',
      'invalid_decimal_number': 'Enter a valid decimal number (max 8 decimal places)',
      'invalid_formula': 'Only numbers and operators (+, -, *, /, . and =) are allowed',
      'invalid_date': 'Invalid date format (YYYY-MM-DD)'
    }
  },
  // pages
  'pages': {
    'account': {
      'toolbar': {
        'settings': 'Configure account',
        'search': 'Search',
        'shared_with': 'Shared account'
      },
      'transaction_list': {
        'none': 'No transactions found',
        'today': 'Today',
        'yesterday': 'Yesterday',
        'item': {
          'transfer_from': 'Transfer from ',
          'transfer_to': 'Transfer to '
        },
        'action': { 'add_transaction': 'Add transaction' }
      },
      'preview_transaction_modal': {
        'header': 'Details',
        'type': {
          'expense': 'Expense',
          'income': 'Income',
          'transfer': 'Transfer'
        },
        'recipient': { 'label': 'Recipient' },
        'sender': { 'label': 'Sender' },
        'category': { 'label': 'Category' },
        'description': { 'label': 'Notes' },
        'payee': { 'label': 'Payee' },
        'tags': { 'label': 'Tags' },
        'author': { 'label': 'Author' },
        'created_at': { 'label': 'Date' }
      },
      'delete_transaction_modal': { 'question': 'Are you sure you want to delete this transaction?' }
    },
    'settings': {
      'settings': {
        'header': 'Settings',
        'header_desktop': 'Service settings',
        'menu_item': 'Settings',
        'logout': 'Logout',
        'groups': {
          'service': 'Service',
          'user_interface': 'User Interface'
        }
      },
      'sync': { 'menu_item': 'Full sync' },
      'accounts': {
        'menu_item': 'Accounts and Folders',
        'header': 'Accounts and Folders',
        'list_empty_create': 'Add',
        'list_empty_new_account': 'new account',
        'list_actions': { 'access': 'Access control' },
        'create_folder': 'Create folder',
        'create_folder_modal': { 'header': 'Create new folder' },
        'update_folder_modal': { 'header': 'Change name' },
        'delete_folder_modal': {
          'title': 'Delete folder?',
          'question': 'Do you want to delete the folder «{folder}»?'
        },
        'delete_account_modal': { 'question': 'Are you sure you want to remove the account «{account}»?' },
        'preview_account_modal': {
          'header': 'Account details',
          'shared_access': 'Shared',
          'access': {
            'label': 'Access',
            'no_shared_access': 'No access to this account'
          },
          'action': { 'shared_access': 'Access control' }
        }
      },
      'currency': { 'menu_item': 'Default currency' },
      'themes': { 'menu_item': 'Color scheme' },
      'language': { 'menu_item': 'Language' },
      'import_csv': { 'menu_item': 'Import CSV' },
      'export_csv': {
        'menu_item': 'Export CSV',
        'error': 'Failed to export transactions'
      }
    },
  },
  // modals
  'modals': {
    'transaction': {
      'transaction_type': {
        'expense': 'Expense',
        'transfer': 'Transfer',
        'income': 'Income'
      },
      'create_form': { 'header': 'Add transaction' },
      'update_form': { 'header': 'Update transaction' },
      'form': {
        'amount': {
          'label': 'Enter amount',
          'validation': { 'required_field': 'Amount is required' }
        },
        'amount_recipient': {
          'label': 'Will be exchanged',
          'validation': { 'required_field': 'Recipient amount is required' }
        },
        'category': {
          'label': 'Category',
          'validation': { 'required_field': 'Category is required' }
        },
        'payee': {
          'expense': 'Recipient',
          'income': 'Sender'
        },
        'options': { 'header': 'Advanced' },
        'description': {
          'label': 'Notes',
          'placeholder': 'Enter notes'
        },
        'from': { 'label': 'From' },
        'to': { 'label': 'To' }
      },
      'dialog': {
        'new_tag': {
          'header': 'Tag name is required',
          'name': {
            'label': 'Tag',
            'validation': { 'required_field': 'Required field' }
          }
        }
      }
    },
    'account': {
      'create_form': { 'header': 'New account' },
      'update_form': { 'header': 'Update account' },
      'form': { 'icon': { 'label': 'Icon' } }
    },
    'export_csv': {
      'header': 'Export CSV'
    },
    'import_csv': {
      'header': 'Import CSV',
      'none': 'None',
      'file': {
        'label': 'CSV File',
        'hint': 'Maximum file size: 10 MB'
      },
      'mapping': {
        'description': 'Map the columns from your CSV file to the transaction fields. Required fields are marked with an asterisk (*).'
      },
      'amount_mode': {
        'label': 'Amount Format',
        'single': 'Single amount column',
        'dual': 'Separate inflow/outflow columns',
        'switch_to_single': 'Switch to single amount',
        'switch_to_dual': 'Split into inflow/outflow'
      },
      'switch_to_manual': 'Pick manually',
      'switch_to_csv': 'Pick CSV column',
      'fields': {
        'account': 'Account',
        'date': 'Date',
        'amount': 'Amount',
        'amount_hint': 'Use if your CSV has one column with positive/negative values',
        'amount_inflow': 'Amount (Inflow)',
        'amount_inflow_hint': 'Income, deposits, credits',
        'amount_outflow': 'Amount (Outflow)',
        'amount_outflow_hint': 'Expenses, withdrawals, debits',
        'category': 'Category',
        'description': 'Description',
        'description_placeholder': 'Enter description for all transactions',
        'payee': 'Payee',
        'tag': 'Tag'
      },
      'success': 'Transactions imported successfully',
      'success_count': '{count} transaction(s) imported successfully | {count} transactions imported successfully',
      'partial_success': 'Imported {imported} transaction(s), {failed} failed',
      'error': 'Failed to import transactions. Please check your file and try again.',
      'errors_detail': 'Errors',
      'row': 'Row',
      'and_more': 'and {count} more error(s)'
    },
    'import_result': {
      'success_title': 'Import Successful',
      'partial_success_title': 'Import Partially Successful',
      'error_title': 'Import Failed',
      'imported': '{count} transaction(s) imported | {count} transactions imported',
      'failed': '{count} transaction(s) failed | {count} transactions failed',
      'errors_detail': 'Error Details',
      'row': 'Row',
      'rows': 'Rows',
      'more': 'more',
      'and_more': 'and {count} more error(s)'
    },
    'sort': {
      'header': 'Sorting',
      'mode': {
        'alphabet': {
          'asc': 'Alphabetically (A-Z)',
          'desc': 'Alphabetically (Z-A)'
        },
        'count': {
          'desc': 'By usage frequency (1..0)',
          'asc': 'By usage frequency (0..1)'
        }
      }
    }
  },
  'modules': {
    'app': {
      'modal': {
        'change_language': { 'header': 'Language' },
        'self_hosted': { 'information': 'You can use the service on your hosting. Just enter the server address.' },
        'loading': { 'data_loading': 'Loading details' },
      },
    },
    'user': {
      'modal': {
        'sign_in': { 'label': 'Authentication...' },
        'sign_in_failed': {
          'header': 'Login failed',
          'information': 'Check that the email and password are entered correctly. If errors persist, contact support.'
        },
        'access_recovery': {
          'header': 'Restore access',
          'information': 'To restore access to your account, enter the email address used during registration.',
          'instruction': 'A security code has been sent to the email address. Enter the code in the field below and provide a new password.',
        },
        'sign_up': { 'label': 'Registration...' },
        'sign_up_failed': {
          'header': 'Registration error',
          'information': 'Ensure that the information is entered correctly. If errors persist, contact support.'
        },
        'sign_out': {
          'title': 'Logout',
          'question': 'Are you sure you want to log out?',
          'action': {
            'logout': 'Logout',
            'cancel': 'Stay'
          }
        },
        'change_password_success': { 'text': 'You have successfully changed your password.' },
        'change_password_error': {
          'header': 'Change password error',
          'text': 'An error occurred while changing the password; please check the information or try again later.'
        },
        'change_password_loading': { 'label': 'Please wait' }
      },
      'form': {
        'user': {
          'name': {
            'label': 'Name',
            'placeholder': 'Your name',
            'validation': {
              'required_field': 'Required field',
              'invalid_name': 'Enter your name'
            }
          },
          'email': {
            'label': 'E-mail',
            'placeholder': 'Your email',
            'validation': {
              'required_field': 'Required field',
              'invalid_email': 'Enter a valid email'
            }
          },
          'code': {
            'label': 'Code',
            'placeholder': 'Confirmation code',
            'validation': {
              'required_field': 'Required field',
              'invalid_code': 'Enter a valid code',
            }
          },
          'password': {
            'label': 'Password',
            'placeholder': 'Password',
            'validation': {
              'required_field': 'Required field',
              'invalid_password': 'Password must be at least 4 characters'
            }
          },
          'password_retry': {
            'label': 'Confirm password',
            'placeholder': 'Re-enter password',
            'validation': {
              'required_field': 'Required field',
              'invalid_password': 'Re-enter password',
              'not_equals': 'Passwords do not match'
            }
          },
          'server_host': {
            'self_hosted': 'Custom server',
            'label': 'Server address',
            'placeholder': 'https://',
            'validation': {
              'required_field': 'Required field',
              'invalid_url': 'Enter a valid server address'
            }
          }
        },
        'sign_in': {
          'action': {
            'sign_in': 'Sign in',
            'forget_password': 'Forgot password?'
          }
        },
        'sign_up': { 'action': { 'sign_up': 'Sign Up' } },
        'access_recovery': {
          'action': {
            'recover': { 'label': 'Restore' },
            'change_password': { 'label': 'Change password' }
          }
        },
        'change_password': {
          'password': {
            'label': 'Current password',
            'placeholder': 'Enter password',
            'validation': { 'invalid_password': 'Enter current password' }
          },
          'new_password': {
            'label': 'New password',
            'placeholder': 'Enter a new password',
            'validation': { 'not_equals': 'New password must differ from the old password' }
          },
          'new_password_retry': {
            'label': 'Confirm new password',
            'placeholder': 'Re-enter new password',
            'validation': { 'not_equals': 'Passwords do not match' }
          },
          'submit': { 'label': 'Change password' }
        }
      },
      'page': {
        'sign_in': {
          'header': 'Login',
        },
        'sign_up': {
          'header': 'Sign up',
          'privacy': {
            'text': 'By registering, you agree to our <a href="https://econumo.com/docs/legal/privacy-policy/" target="_blank" class=\'register-form-privacy-link\'>Privacy Policy</a> and <a href="https://econumo.com/docs/legal/terms-of-service/" target="_blank" class=\'register-form-privacy-link\'>Terms of Service</a>'
          },
          'paywall': {
            'header': 'Ready to start using Econumo?',
            'text': 'Make a one-time payment by clicking the button below to get started.',
            'next_steps': 'As soon as we receive your payment, we will send your login credentials to the provided email address.',
            'action': 'Sign Up'
          }
        },
        'settings': {
          'profile': {
            'menu_item': 'Profile',
            'header': 'User profile',
            'groups': { 'security': 'Security' },
            'change_password': {
              'menu_item': 'Change password',
              'header': 'Change password'
            },
          },
        }
      },
      'pages': {
        'onboarding': {
          'header': 'Onboarding',
          'title': 'Welcome to Econumo!',
          'complete': 'Complete onboarding',
          'add_account': 'Add an account',
          'import_transactions': 'Import Transactions',
          'user_guide': {
            'accounts': 'User guide: accounts',
            'transactions': 'User guide: transactions',
            'classifications': 'User guide: classifications',
            'user_profile': 'User guide: user profile',
            'shared_access': 'User guide: shared access',
            'budgets': 'User guide: budgets',
          },
        }
      }
    },
    'export_csv': {
      'modal': {
        'export_csv_form': {
          'header': 'Export CSV',
          'accounts': 'Select Accounts',
          'select_all': 'Select All',
          'deselect_all': 'Deselect All'
        }
      }
    },
    'budget': {
      'modal': {
        'budget_form': { 'accounts': 'Accounts' },
        'create_budget_form': { 'header': 'Create a budget' },
        'update_budget_form': { 'header': 'Update the budget' },
        'create_folder_form': { 'header': 'Create a folder' },
        'update_folder_form': { 'header': 'Rename the folder' },
        'create_envelope_form': { 'header': 'Create an envelope' },
        'update_envelope_form': { 'header': 'Update the envelope' },
        'change_element_currency_form': { 'header': 'Change currency' },
        'delete_envelope': {
          'header': 'Delete the envelope?',
          'question': 'Are you sure you want to delete this envelope?'
        },
        'set_limit_form': { 'header': 'Set limit' },
        'generic_error': {
          'header': 'Error Occurred',
          'description': 'Please try again. If the error persists, report the issue.'
        },
        'access_error': {
          'header': 'Access Denied',
          'description': 'You need to accept the invitation before accessing this budget.'
        },
        'expense_widget': {
          'header': 'Outflow vs. Total',
          'conversion_rate': 'The average rate for {period} is 1 {defaultCurrency} = {rate}'
        }
      },
      'form': {
        'budget': {
          'name': {
            'label': 'Name',
            'placeholder': 'My budget',
            'validation': {
              'required_field': 'Required field',
              'invalid_name': 'Budget name must be between 3 and 64 characters'
            }
          },
          'folder_name': {
            'label': 'Folder name',
            'placeholder': 'Provide a folder name',
            'validation': {
              'required_field': 'Required field',
              'invalid_name': 'Folder name must be between 3 and 64 characters'
            }
          }
        },
        'budget_envelope': {
          'name': {
            'label': 'Name',
            'placeholder': '',
            'validation': {
              'required_field': 'Required field',
              'invalid_name': 'Envelope name must be between 3 and 64 characters'
            }
          },
          'currency': {
            'label': 'Currency',
            'validation': {
              'required_field': 'Required field',
            }
          },
          'categories': {
            'label': 'Categories',
          },
          'icon': {
            'label': 'Icon',
          },
          'status': {
            'label': 'Status',
            'option': {
              'active': 'Active',
              'archive': 'Archived',
            }
          }
        },
        'budget_limit': {
          'limit': {
            'label': 'Limit',
            'validation': {
              'invalid_number': 'Invalid number',
              'required_field': 'Required field',
              'invalid_formula': 'Invalid formula'
            }
          }
        }
      },
      'page': {
        'budget': {
          'empty': {
            'header': 'Budget',
            'no_budget': 'You haven’t created a budget yet.',
            'description': 'Create or accept an invitation to a budget to start managing your finances.',
            'create_budget': 'Create a budget',
            'initial_setup': 'To create a budget, you need to create an account and add at least one category.',
            'create_account': 'Create an account',
          },
          'settings': {
            'button': 'Configure',
            'menu': {
              'edit': 'Budget details',
              'edit_structure': 'Edit structure',
              'edit_structure_done': 'Done Editing',
              'budget_list': 'Open budget list',
            }
          },
          'structure': {
            'no_folder': 'Default folder',
            'in_archive': 'Archived',
            'tab': {
              'budgeted': 'Budget',
              'spent': 'Spent',
              'available': 'Available'
            },
            'action': {
              'create_folder': 'Create a folder',
              'delete_folder': 'Delete a folder',
            },
            'empty_folder': {
              'note': 'Folder is empty. Move a category/tag/envelope or create a new envelope.'
            },
            'element': {
              'action': {
                'change_currency': 'Change currency',
              }
            },
            'total': {
              'name': 'Total',
              'expenses': 'Total expenses',
            }
          }
        },
        'settings': {
          'menu_item': 'Budgets',
          'header': 'Budgets',
          'create_budget': 'Create a new budget',
          'edit_modal': { 'header': 'Update the budget' },
          'create_modal': { 'header': 'Create a new budget' },
          'delete_modal': {
            'title': 'Delete the budget?',
            'question': 'Are you sure you want to delete {name}?'
          },
          'decline_access_modal': {
            'title': 'Decline access to the budget?',
            'question': 'Are you sure you want to decline access to the budget {name}?'
          },
          'not_accepted': 'not accepted',
          'level': {
            'guest': 'View only',
            'user': 'Regular access',
            'admin': 'Full control',
          },
          'list_actions': {
            'access': 'Access control',
            'go_to': 'Go to the budget',
            'select': 'Select the budget'
          }
        }
      }
    },
    'classifications': {
      'categories': {
        'pages': {
          'settings': {
            'menu_item': 'Categories',
            'header': 'Categories',
            'archived_item': 'Archived (inactive)',
            'create_category': 'Create category',
          }
        },
        'elements': {},
        'modals': {
          'replace': {
            'header': 'Replace with',
            'note': 'The original category will be deleted and replaced with the selected one'
          },
          'delete': {
            'title': 'Delete category?',
            'question': 'Delete category {name}?'
          },
          'edit': { 'header': 'Edit' },
          'create': { 'header': 'New category' }
        },
        'forms': {
          'category': {
            'type': {
              'income': 'Income',
              'expense': 'Expense'
            },
            'name': {
              'label': 'Name',
              'placeholder': 'Enter name',
              'validation': {
                'required_field': 'Required field',
                'invalid_name': 'The category name must be between 3 and 64 characters'
              }
            },
            'icon': { 'label': 'Icon' }
          },
        }
      },
      'tags': {
        'pages': {
          'settings': {
            'menu_item': 'Tags',
            'header': 'Tags',
            'create_tag': 'Create a new tag',
            'archived_item': 'Archived (inactive)',
          }
        },
        'elements': {},
        'modals': {
          'edit': { 'header': 'Edit tag' },
          'create': { 'header': 'Create a new tag' },
          'delete': {
            'title': 'Delete tag?',
            'question': 'Are you sure you want to delete the tag {name}?'
          }
        },
        'forms': {
          'tag': {
            'name': {
              'label': 'Name',
              'validation': {
                'required_field': 'Required field',
                'invalid_name': 'The tag name must be between 3 and 64 characters'
              }
            }
          },
        }
      },
      'payees': {
        'pages': {
          'settings': {
            'menu_item': 'Payees (senders, recipients)',
            'header': 'Payees',
            'create_payee': 'Create a new payee',
            'archived_item': 'Archived (inactive)',
          }
        },
        'elements': {},
        'modals': {
          'edit': { 'header': 'Update payee' },
          'create': { 'header': 'Create a new payee' },
          'delete': {
            'title': 'Delete payee?',
            'question': 'Are you sure you want to delete {name}?'
          }
        },
        'forms': {
          'payee': {
            'name': {
              'label': 'Name',
              'validation': {
                'required_field': 'Required field',
                'invalid_name': 'Payee name must be between 3 and 64 characters'
              }
            }
          }
        }
      }
    },
    'connections': {
      'pages': {
        'settings': {
          'menu_item': 'Shared access',
          'header': 'Shared access',
          'generate_invite': 'Create an invitation',
          'accept_invite': 'Accept an invitation',
          'info': {
            'header': '<b>Econumo CE</b> does not support user connection management and offers two modes suitable for families and individuals:',
            'list': {
              'all_users_connected': 'All users connected automatically',
              'all_users_connected_description': 'Ideal for families, allowing easy sharing of accounts or budgets.',
              'no_connections': 'No connections between users',
              'no_connections_description': 'No shared accounts or budgets.',
            },
            'link': 'Read more',
            'link_url': 'https://econumo.com/docs/edition/'
          }
        }
      },
      'accounts': {
        'roles': {
          'owner': 'Owner',
          'admin': 'Full control',
          'user': 'Manage transactions',
          'guest': 'View only',
          'no_access': 'No access'
        }
      },
      'budgets': {
        'roles': {
          'owner': 'Owner',
          'admin': 'Full control',
          'user': 'Manage budget',
          'guest': 'View only',
          'no_access': 'No access'
        }
      },
      'modals': {
        'delete_connection': { 'question': 'Are you sure you want to delete shared access from {name}?' },
        'generate_invite': {
          'instruction': 'To connect with another user, share this code with your partner. The code is valid for 5 minutes.',
          'code': { 'label': 'Invitation code' }
        },
        'accept_invite': {
          'label': 'Accept an invitation',
          'instruction': 'Ask your partner for the invitation code and enter it below. The code is valid for 5 minutes.',
          'code': { 'label': 'Enter the code' }
        },
        'preview_connection': {
          'accounts': 'Shared accounts',
          'budgets': 'Shared budgets',
          'budgets_empty': 'No shared budgets',
          'accounts_empty': 'No shared accounts',
          'shared_with_you': 'Shared with you',
          'your_account': 'Your account',
          'your_budget': 'Your budget',
          'tap_to_manage': 'Click to manage access',
        },
        'decline_access': {
          'decline_access': 'Decline access',
        },
        'share_access': {
          'level': {
            'guest': 'View only',
            'user': 'Regular access',
            'admin': 'Full control',
            'owner': 'Owner',
            'no_access': 'No access'
          },
          'not_accepted': 'not accepted',
          'revoke_access': 'Revoke access',
          'list_empty': 'No connections found',
          'tap_to_share': 'Click on a user to share access',
          'choose_access_level': 'Choose what access level to grant',
          'select_user': 'Select user {name}'
        },
      },
      'forms': {
        'invitation_code': {
          'validation': { 'required_field': 'Required field' }
        }
      }
    }
  }
}
