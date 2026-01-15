<?php

declare(strict_types=1);

namespace App\Tests\api\v1\transaction;

use Codeception\Exception\ModuleException;
use App\Tests\ApiTester;
use Codeception\Util\HttpCode;
use PHPUnit\Framework\Assert;

class ImportTransactionListCest
{
    private string $url = '/api/v1/transaction/import-transaction-list';

    /**
     * @throws ModuleException
     */
    public function requestShouldReturn401ResponseCode(ApiTester $I): void
    {
        $I->sendPOST($this->url, []);
        $I->seeResponseCodeIs(HttpCode::UNAUTHORIZED);
    }

    /**
     * @throws ModuleException
     */
    public function requestShouldReturnGroupedErrors(ApiTester $I): void
    {
        $I->amAuthenticatedAsJohn();

        $mapping = json_encode([
            'account' => 'Account',
            'date' => 'Date',
            'amount' => 'Amount',
            'category' => 'Category',
            'description' => 'Description',
        ], JSON_THROW_ON_ERROR);

        $I->sendPOST(
            $this->url,
            ['mapping' => $mapping],
            ['file' => codecept_data_dir('import_transactions_invalid_category.csv')]
        );

        $I->seeResponseCodeIs(HttpCode::OK);

        $response = json_decode($I->grabResponse(), true, 512, JSON_THROW_ON_ERROR);
        $errorMessage = 'Category name must be 3-64 characters';

        Assert::assertSame([4, 5], $response['data']['errors'][$errorMessage] ?? null);
    }
}
