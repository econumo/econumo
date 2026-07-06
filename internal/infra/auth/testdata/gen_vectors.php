<?php
// Generates authoritative crypto golden vectors from the live Symfony hasher
// and a faithful EncodeService transcription. Run inside a PHP container with
// the project vendor/ mounted. Output is JSON consumed by the Go golden tests.
//
//   docker run --rm -v "$PWD":/app -w /app php:8.3-cli \
//     php go/internal/infra/auth/testdata/gen_vectors.php > go/internal/infra/auth/testdata/vectors.json

declare(strict_types=1);

// Require ONLY the hasher class + its trait/interfaces directly, bypassing the
// composer autoloader's PHP-version platform check (we just need this one class).
$base = __DIR__ . '/../../../../../vendor/symfony/password-hasher';
require $base . '/Exception/ExceptionInterface.php';
require $base . '/Exception/InvalidPasswordException.php';
require $base . '/Exception/LogicException.php';
require $base . '/PasswordHasherInterface.php';
require $base . '/LegacyPasswordHasherInterface.php';
require $base . '/Hasher/CheckPasswordLengthTrait.php';
require $base . '/Hasher/MessageDigestPasswordHasher.php';

use Symfony\Component\PasswordHasher\Hasher\MessageDigestPasswordHasher;

// Mirror EncodeService.php (deterministic parts: hash; encode uses a random IV
// so we instead emit a fixed ciphertext we can DECODE, produced with a known IV).
function svc_hash(string $value, string $salt): string {
    return md5($value . $salt);
}

// Deterministic encode with an explicit IV so the vector is reproducible.
function svc_encode_fixedIV(string $value, string $salt, string $iv): string {
    $cipher = 'AES-128-CBC';
    $ct = openssl_encrypt($value, $cipher, $salt, OPENSSL_RAW_DATA, $iv);
    $hmac = hash_hmac('sha256', $ct, $salt, true);
    return base64_encode($iv . $hmac . $ct);
}

$hasher = new MessageDigestPasswordHasher('sha512', true, 500);

$passwords = [
    ['password' => 'secret123', 'salt' => 'a3f1c9d2e4b5067890abcdef1234567890abcdef'],
    ['password' => 'hunter2',   'salt' => '0011223344556677889900aabbccddeeff001122'],
    ['password' => '',          'salt' => 'deadbeefdeadbeefdeadbeefdeadbeefdeadbeef'],
    ['password' => 'пароль🔒',  'salt' => 'cafebabecafebabecafebabecafebabecafebabe'],
];
$pwVectors = [];
foreach ($passwords as $p) {
    $pwVectors[] = [
        'password' => $p['password'],
        'salt'     => $p['salt'],
        'hash'     => $hasher->hash($p['password'], $p['salt']),
    ];
}

// EncodeService vectors use a 16-byte salt (AES-128 key) per production reality.
$dataSalt = '0123456789abcdef'; // exactly 16 bytes
$iv = "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f";
$emails = ['john@example.com', 'JOHN@Example.com', 'тест@пример.рф'];
$hashVectors = [];
$encodeVectors = [];
foreach ($emails as $e) {
    $hashVectors[] = ['value' => strtolower($e), 'salt' => $dataSalt, 'hash' => svc_hash(strtolower($e), $dataSalt)];
    $encodeVectors[] = ['plaintext' => $e, 'salt' => $dataSalt, 'iv' => bin2hex($iv), 'ciphertext' => svc_encode_fixedIV($e, $dataSalt, $iv)];
}

echo json_encode([
    'password_hasher' => $pwVectors,
    'identifier_hash' => $hashVectors,
    'encode'          => $encodeVectors,
], JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES), "\n";
