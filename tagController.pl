#!/usr/bin/env perl
# =======================================================================================
#
#      Author:   Jan Eitzinger (je), jan.eitzinger@fau.de
#      Copyright (c) 2019 RRZE, University Erlangen-Nuremberg
#
#      Permission is hereby granted, free of charge, to any person obtaining a copy
#      of this software and associated documentation files (the "Software"), to deal
#      in the Software without restriction, including without limitation the rights
#      to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
#      copies of the Software, and to permit persons to whom the Software is
#      furnished to do so, subject to the following conditions:
#
#      The above copyright notice and this permission notice shall be included in all
#      copies or substantial portions of the Software.
#
#      THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
#      IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
#      FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
#      AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
#      LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
#      OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
#      SOFTWARE.
#
# =======================================================================================

use strict;
use warnings;
use utf8;

use File::Slurp;
use Data::Dumper;
use JSON::MaybeXS qw(encode_json decode_json);

my $basedir = $ARGV[0];
my $basedir = './data';


my ($TS, $TE);
my $counter = 0;

open(my $fhn, '>:encoding(UTF-8)', './jobIds-tagged.txt')
    or die "Could not open file  $!";
open(my $fh, '<:encoding(UTF-8)', './jobIds.txt')
    or die "Could not open file  $!";
$TS = time();

while ( <$fh> ) {

    my $line = $_;
    my ($jobID, $system) = split '.', $line;
    $counter++;

    # my $json = read_file($jobDirectory.'/data.json');
    # my $data = decode_json $json;
    my $json = read_file($jobDirectory.'/meta.json');
    my $meta = decode_json $json;

    my $footprint = $meta->{statistics};

    if ( $footprint->{flops_any}->{max} < 2.0 and  $footprint->{mem_bw}->{max} < 2.0 ){
        print $fhn $jobID;
    }

    if ( $counter == 20 ) {
        $TE = time();
        my $rate = $counter/($TE-$TS);
        $counter = 0;
        print "Processing $rate jobs per second\n";
        $TS = $TE;
    }
}
close $fh;
close $fhn;
