<?php
declare(strict_types = 1);

namespace Sourcegraph\BuildServer;

use LanguageServer\FilesFinder\{FilesFinder, FileSystemFilesFinder};
use Webmozart\PathUtil\Path;
use Webmozart\Glob\Glob;
use Sabre\Uri;
use Sabre\Event\Promise;
use function Sabre\Event\coroutine;

class DependencyAwareFilesFinder implements FilesFinder
{
    /**
     * @var FilesFinder
     */
    private $wrappedFilesFinder;

    /**
     * @var FileSystemFilesFinder
     */
    private $fileSystemFilesFinder;

    /**
     * The temporary folder where the dependencies are actually installed
     *
     * @var string
     */
    private $dependencyDir;

    /**
     * The URI where the vendor directory would be in the workspace
     *
     * @var string
     */
    private $composerJsonDir;

    /**
     * @var string
     */
    private $rootPath;

    /**
     * @var \stdClass
     */
    private $composerLock;

    /**
     * @param FilesFinder $wrappedFilesFinder The FilesFinder to fallback to
     * @param string      $rootPath           The workspace root path
     * @param string      $composerJsonDir    The URI where the composer.json and vendor directory would be in the workspace
     * @param string      $dependencyDir      The temporary folder path where the dependencies are actually installed
     * @param stdClass    $composerLock       The parsed composer.lock
     */
    public function __construct(FilesFinder $wrappedFilesFinder, string $rootPath, string $composerJsonDir, string $dependencyDir, \stdClass $composerLock)
    {
        $this->wrappedFilesFinder = $wrappedFilesFinder;
        $this->fileSystemFilesFinder = new FileSystemFilesFinder;
        $this->rootPath = $rootPath;
        $this->dependencyDir = $dependencyDir;
        $this->composerJsonDir = $composerJsonDir;
        $this->composerLock = $composerLock;
    }

    public function find(string $glob): Promise
    {
        return coroutine(function () use ($glob) {
            list($sourceResults, $dependencyResults) = yield Promise\all([
                // Glob workspace
                $this->wrappedFilesFinder->find($glob),
                // Glob dependencies
                coroutine(function () use ($glob) {
                    // Check if files inside the vendor path would match the glob
                    $composerJsonDirParts = Uri\parse($this->composerJsonDir);
                    $vendorPath = Path::join($composerJsonDirParts['path'], 'vendor');
                    if (!Glob::match($vendorPath, dirname($glob))) {
                        return [];
                    }
                    $depsGlob = Path::makeAbsolute(Path::makeRelative($glob, $composerJsonDirParts['path']), $this->dependencyDir);
                    $dependencyResults = yield $this->fileSystemFilesFinder->find($depsGlob);
                    $sourcegraphUris = [];
                    // Rewrite dependency temporary folder URIs to Sourcegraph repository URI
                    foreach ($dependencyResults as $dependencyUri) {
                        // Get package name from URI
                        if (preg_match('/\/vendor\/([^\/]+\/[^\/]+)\//', $dependencyUri, $matches) === 0) {
                            continue;
                        }
                        $packageName = $matches[1];
                        foreach ($this->composerLock->packages as $package) {
                            if ($package->name !== $packageName) {
                                continue;
                            }
                            if (!isset($package->source) || $package->source->type !== 'git' ) {
                                // Not supported atm
                                break;
                            }
                            $dependencyPath = Uri\parse($dependencyUri)['path'];
                            $relativeDependencyPath = Path::makeRelative($dependencyPath, $this->dependencyDir);
                            $newUri = $composerJsonDirParts;
                            $newUri['path'] = Path::join($newUri['path'], $relativeDependencyPath);
                            $sourcegraphUris[] = Uri\build($newUri);
                            break;
                        }
                    }
                    return $sourcegraphUris;
                })
            ]);
            return array_merge($sourceResults, $dependencyResults);
        });
    }
}
