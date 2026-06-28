<?php
// Generates authoritative DecimalNumber golden vectors from the REAL PHP
// DecimalNumber value object (bcmath, scale 8). Output is JSON consumed by the
// Go golden test (decimal_test.go). Run inside a PHP container with bcmath:
//
//   docker run --rm -v "$PWD":/app -w /app php:8.3-cli \
//     php go/internal/domain/shared/vo/testdata/gen_decimal_vectors.php \
//     > go/internal/domain/shared/vo/testdata/decimal_vectors.json
//
// (php:8.3-cli ships bcmath built in.)

declare(strict_types=1);

// Load ONLY the DecimalNumber class + the ValueObjectInterface it implements,
// bypassing the composer autoloader's platform checks.
$base = __DIR__ . '/../../../../../../src/EconumoBundle/Domain/Entity/ValueObject';
require $base . '/ValueObjectInterface.php';
require $base . '/DecimalNumber.php';

use App\EconumoBundle\Domain\Entity\ValueObject\DecimalNumber;

$pairs = [
    ['0.92', '95'], ['100', '3'], ['1', '3'], ['2', '3'],
    ['10.5', '2.5'], ['0.1', '0.2'], ['1.23456789', '2'],
    ['-5.5', '2'], ['5.5', '-2'], ['-5.5', '-2'],
    ['1234.56789012', '0.00000003'], ['999999.99999999', '1'],
    ['0.00000001', '0.00000002'], ['7', '0.13'],
    ['86.7', '1.0852'], ['100', '1.0852'], ['250.25', '0.8765'],
];

$rows = [];
foreach ($pairs as [$a, $b]) {
    $da = new DecimalNumber($a);
    $db = new DecimalNumber($b);
    $rows[] = [
        'a'   => $da->getValue(),
        'b'   => $db->getValue(),
        'add' => $da->add($db)->getValue(),
        'sub' => $da->sub($db)->getValue(),
        'mul' => $da->mul($db)->getValue(),
        'div' => $db->isZero() ? null : $da->div($db)->getValue(),
    ];
}

// Rounding vectors: value + precision -> rounded.
$roundCases = [
    ['1.235', 2], ['1.245', 2], ['1.005', 2], ['2.5', 0], ['3.5', 0],
    ['-2.5', 0], ['-1.235', 2], ['0.123456785', 8], ['86.74', 2],
    ['94.0852', 2], ['94.0852', 0], ['0.5', 0], ['-0.5', 0],
    ['123.456789', 4], ['999.995', 2],
];
$rounds = [];
foreach ($roundCases as [$v, $p]) {
    $rounds[] = ['value' => (new DecimalNumber($v))->getValue(), 'precision' => $p, 'result' => (new DecimalNumber($v))->round($p)->getValue()];
}

echo json_encode(['arith' => $rows, 'round' => $rounds], JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES), "\n";
