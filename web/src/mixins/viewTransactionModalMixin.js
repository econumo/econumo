import _ from 'lodash';

export const viewTransactionModalMixin = {
  data() {
    return {
      previewTransactionModal: {
        isOpened: false,
        transaction: null,
        account: null,
        accountRecipient: null
      },
    }
  },
  methods: {
    openPreviewTransactionModal: function (transactionId) {
      const transaction = _.cloneDeep(_.find(this.transactions, {id: transactionId}));
      this.previewTransactionModal.transaction = transaction;
      this.previewTransactionModal.account =
        transaction.account || _.find(this.accounts, {id: transaction.accountId}) || null;
      if (transaction.accountRecipientId) {
        this.previewTransactionModal.accountRecipient =
          transaction.accountRecipient || _.find(this.accounts, {id: transaction.accountRecipientId}) || null;
      } else {
        this.previewTransactionModal.accountRecipient = null;
      }
      this.previewTransactionModal.isOpened = true;
    },
  }
}
