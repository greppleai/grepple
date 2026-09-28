<!doctype html>
<html><body>
<?php
namespace App\Service;

use App\Model\User;

/** Formats a user. */
class Formatter {
    public string $label;
    public function format(User $user): string {
        return helper($user->name);
    }

    private function unused(): void {
        echo "unrelated";
    }
}

function helper(string $value): string {
    return strtoupper($value);
}
?>
</body></html>
