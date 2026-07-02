<?php
// verify_go_token.php — proves Lexik (PHP) would accept a Go-issued token.
// Reads a compact JWT from argv[1], splits it, and runs openssl_verify with the
// repo public key (exactly how Lexik validates an incoming token's signature).
declare(strict_types=1);

$jwt = $argv[1] ?? '';
$pub = openssl_pkey_get_public(file_get_contents('config/jwt/public.pem'));
if ($pub === false) { fwrite(STDERR, "load pub failed\n"); exit(2); }

[$h, $p, $s] = explode('.', $jwt);
$sig = base64_decode(strtr($p, '', '') === '' ? '' : strtr($s, '-_', '+/') . str_repeat('=', (4 - strlen($s) % 4) % 4));
$signingInput = $h . '.' . $p;

$ok = openssl_verify($signingInput, $sig, $pub, OPENSSL_ALGO_SHA256);
echo $ok === 1 ? "VALID\n" : "INVALID($ok)\n";

// Also decode and print payload so we can eyeball the claim shape.
$payload = json_decode(base64_decode(strtr($p, '-_', '+/')), true);
echo json_encode($payload, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES) . "\n";
exit($ok === 1 ? 0 : 1);
