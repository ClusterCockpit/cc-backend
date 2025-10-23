#!/usr/bin/env perl

use strict;
use warnings;
use utf8;

my %INFO;
my %DOMAINS;

my $SMT;
my $numMemoryDomains;
$DOMAINS{socket} = [];
$DOMAINS{memoryDomain} = [];
$DOMAINS{core} = [];
$DOMAINS{gpu} = [];

my $gpuID=-1;
my $id;

# Step 1 : Extract system information
my $topo = `likwid-topology -O -G`;
$INFO{numGPUs} = 0;

foreach my $ln (split("\n", $topo)) {
    if ( $ln =~ /^STRUCT,NUMA Topology ([0-9]+)/ ) {
        $id = $1;
    }
    if ( $ln =~ /^Processors/ ) {
        my @fields = split(",", $ln);
        shift @fields;
        $DOMAINS{memoryDomain}[$id] = [ @fields ];
    }
    if ( $ln =~ /^STRUCT,Cache Topology L1/ ) {
        $id = -1;
    }
    if ( $ln =~ /^Cache groups/ ) {
        if ( $id == -1 ) {
            my @fields = split(",", $ln);
            shift @fields;
            my $i = 0;
            foreach my $core ( @fields ) {
                $DOMAINS{core}[$i++] = [ split(" ", $core) ];
            }
            $id = 0;
        }
    }
    if ( $ln =~ /^ID:/ ) {
        my @fields = split(",", $ln);
        $gpuID = $fields[1];
    }
    if ( $gpuID >= 0 ) {
        if ( $ln =~ /^Name:/ ) {
            my @fields = split(",", $ln);
            $DOMAINS{gpu}[$gpuID] = {};
            $DOMAINS{gpu}[$gpuID]{model} = $fields[1];
            if ( $fields[1] =~ /nvidia/i ) {
                $DOMAINS{gpu}[$gpuID]{type} = "Nvidia GPU";
            } elsif  ( $fields[1] =~ /amd/i ) {
                $DOMAINS{gpu}[$gpuID]{type} = "AMD GPU";
            } elsif  ( $fields[1] =~ /intel/i ) {
                $DOMAINS{gpu}[$gpuID]{type} = "Intel GPU";
            }
        }
        if ( $ln =~ /^PCI bus:/ ) {
            my @fields = split(",", $ln);
            $fields[1] =~ s/0x//;
            $DOMAINS{gpu}[$gpuID]{bus} = $fields[1];
        }
        if ( $ln =~ /^PCI domain:/ ) {
            my @fields = split(",", $ln);
            $fields[1] =~ s/0x//;
            $DOMAINS{gpu}[$gpuID]{domain} = $fields[1];
        }
        if ( $ln =~ /^PCI device/ ) {
            my @fields = split(",", $ln);
            $fields[1] =~ s/0x//;
            $DOMAINS{gpu}[$gpuID]{device} = $fields[1];
            $gpuID = -1;
        }
    }
    if ( $ln =~ /^CPU name:/ ) {
        my @fields = split(",", $ln);
        $INFO{processor} = $fields[1];
    }
    if ( $ln =~ /^CPU type/ ) {
        my @fields = split(",", $ln);
        $INFO{family} = $fields[1];
        $INFO{family} =~ s/[\(\)]//g;
    }
    if ( $ln =~ /^Sockets:/ ) {
        my @fields = split(",", $ln);
        $INFO{socketsPerNode} = $fields[1];
    }
    if ( $ln =~ /^Cores per socket:/ ) {
        my @fields = split(",", $ln);
        $INFO{coresPerSocket} = $fields[1];
    }
    if ( $ln =~ /^GPU count:/ ) {
        my @fields = split(",", $ln);
        $INFO{numGPUs} = $fields[1];
    }
    if ( $ln =~ /^Threads per core:/ ) {
        my @fields = split(",", $ln);
        $SMT = $fields[1];
        $INFO{threadsPerCore} = $SMT;
    }
    if ( $ln =~ /^NUMA domains:/ ) {
        my @fields = split(",", $ln);
        $INFO{memoryDomainsPerNode} = $fields[1];
    }
    if ( $ln =~ /^Socket ([0-9]+)/ ) {
        my @fields = split(",", $ln);
        shift @fields;
        $DOMAINS{socket}[$1] = [ @fields ];
    }
}

my $node;
my @sockets;
my @nodeCores;
foreach my $socket ( @{$DOMAINS{socket}} ) {
    push @sockets, "[".join(",", @{$socket})."]";
    push @nodeCores, join(",", @{$socket});
}
$node =  join(",", @nodeCores);
$INFO{sockets} = join(",\n", @sockets);

my @memDomains;
foreach my $d ( @{$DOMAINS{memoryDomain}} ) {
    push @memDomains, "[".join(",", @{$d})."]";
}
$INFO{memoryDomains} = join(",\n", @memDomains);

my @cores;
foreach my $c ( @{$DOMAINS{core}} ) {
    push @cores, "[".join(",", @{$c})."]";
}
$INFO{cores} = join(",", @cores);

my $numCoresPerNode = $INFO{coresPerSocket} * $INFO{socketsPerNode};
my $numCoresPerMemoryDomain = $numCoresPerNode / $INFO{memoryDomainsPerNode};
my $memBw;

my $exp = join(' ',map("-w M$_:1GB:$numCoresPerMemoryDomain:1:$SMT", 0 ... $INFO{memoryDomainsPerNode}-1));
print "Using: $exp\n";
my $out =  `likwid-bench -t clload $exp`;
foreach my $ln ( split("\n", $out) ){
    if ( $ln =~ /MByte\/s:\s+([0-9.]+)/ ) {
        $memBw = my $rounded = int($1/1000 + 0.5);
    }
}

my $flopsScalar;
$out =  `likwid-bench -t peakflops -w N:24kB:$numCoresPerNode`;
foreach my $ln ( split("\n", $out) ){
    if ( $ln =~ /MFlops\/s:\s+([0-9.]+)/ ) {
        $flopsScalar = my $rounded = int($1/1000 + 0.5);
    }
}

my $simd = "";
my $fh;
open($fh,"<","/proc/cpuinfo");
foreach my $ln ( <$fh> ) {
    if ( $ln =~ /flags/ ) {
        if ( $ln =~ /avx2/ ) {
            $simd = '_avx_fma';
        }
        if ( $ln =~ /avx512ifma/ ) {
            $simd = '_avx512_fma';
        }
        last;
    }
}
close $fh;

print "Using peakflops variant $simd\n";
my $flopsSimd;
$out =  `likwid-bench -t peakflops$simd -w N:500kB:$numCoresPerNode`;
foreach my $ln ( split("\n", $out) ){
    if ( $ln =~ /MFlops\/s:\s+([0-9.]+)/ ) {
        $flopsSimd = my $rounded = int($1/1000 + 0.5);
    }
}

if ( $INFO{numGPUs} > 0 ) {
    $INFO{gpus} = "\"accelerators\": [\n";

    my @gpuStr;

    foreach $id ( 0 ... ($INFO{numGPUs}-1) ) {
        my %gpu = %{$DOMAINS{gpu}[$id]};
        my $deviceAddr =  sprintf("%08x:%02x:%02x\.0", hex($gpu{domain}), hex($gpu{bus}), hex($gpu{device}));
        $gpuStr[$id] = <<END
     {
            "id": "$deviceAddr",
            "type": "$gpu{type}",
             "model": "$gpu{model}"
         }
END
    }

    $INFO{gpus} .= join(",\n",@gpuStr);
    $INFO{gpus} .= "]\n";
} else {
    $INFO{gpus} = '';
}


print <<"END";
{
      "name": "<FILL IN>",
      "processorType": "$INFO{processor}",
      "socketsPerNode": $INFO{socketsPerNode},
      "coresPerSocket": $INFO{coresPerSocket},
      "threadsPerCore": $INFO{threadsPerCore},
      "flopRateScalar": {
           "unit": {
               "base": "F/s",
               "prefix": "G"
           },
           "value": $flopsScalar
      },
      "flopRateSimd": {
           "unit": {
               "base": "F/s",
               "prefix": "G"
           },
           "value": $flopsSimd
      },
      "memoryBandwidth": {
           "unit": {
               "base": "B/s",
               "prefix": "G"
           },
           "value": $memBw
      },
      "nodes": "<FILL IN NODE RANGES>",
      "topology": {
          "node": [$node],
          "socket": [
          $INFO{sockets}
          ],
          "memoryDomain": [
          $INFO{memoryDomains}
          ],
          "core": [
          $INFO{cores}
          ]
          $INFO{gpus}
      }
}
END
