<?php
namespace App\Service;

use App\Support\Formatter as Format;

interface Labeler {
    public function label(string $value): string;
}

class Greeter implements Labeler {
    private string $prefix;

    public function __construct(string $prefix) {
        $this->prefix = $prefix;
    }

    public function label(string $value): string {
        return Format::render($this->prefix . $value);
    }
}

function greet(Greeter $greeter): string {
    return $greeter->label('world');
}

function unrelated(): void {
    echo 'elsewhere';
}
