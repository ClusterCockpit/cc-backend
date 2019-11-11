#!/usr/bin/env perl

use strict;
use warnings;
use utf8;

use File::Slurp;
use Data::Dumper;
use JSON::MaybeXS qw(encode_json decode_json);

my $jobDirectory = '../data';

sub gnuplotControl {
    my $jobID = shift;
    my $metricName = shift;
    my $numNodes = shift;
    my $unit = shift;

    my $gpMacros = <<"END";
set terminal png size 1400,768 enhanced font ,12
set output '$jobID-$metricName.png'
set xlabel 'runtime [s]'
set ylabel '[$unit]'
END

    $gpMacros .= "plot '$metricName.dat' u 2 w lines notitle";
    foreach my $col ( 3 ... $numNodes ){
        $gpMacros .= ", '$metricName.dat' u $col w lines notitle";
    }

    open(my $fh, '>:encoding(UTF-8)', './metric.plot')
        or die "Could not open file  $!";
    print $fh $gpMacros;
    close $fh;

    system('gnuplot','metric.plot');
}

sub createPlot {
    my $jobID = shift;
    my $metricName = shift;
    my $metric = shift;
    my $unit = shift;

    my @lines;

    foreach my $node ( @$metric ) {
        my $i = 0;

        foreach my $val ( @{$node->{data}} ){
            $lines[$i++] .= " $val";
        }
    }

    open(my $fh, '>:encoding(UTF-8)', './'.$metricName.'.dat')
        or die "Could not open file  $!";

    my $timestamp = 0;

    foreach my $line ( @lines ) {
        print $fh $timestamp.$line."\n";
        $timestamp += 60;
    }

    close $fh;
    gnuplotControl($jobID, $metricName, $#$metric + 2, $unit);
}

mkdir('./plots');
chdir('./plots');

while ( <> ) {
    my $jobID = $_;
    $jobID =~ s/\.eadm//;
    chomp $jobID;

    my $level1 = $jobID/1000;
    my $level2 = $jobID%1000;
    my $jobpath = sprintf("%s/%d/%03d", $jobDirectory, $level1, $level2);

    my $json = read_file($jobpath.'/data.json');
    my $data = decode_json $json;
    $json = read_file($jobpath.'/meta.json');
    my $meta = decode_json $json;

    createPlot($jobID, 'flops_any', $data->{flops_any}->{series}, $data->{flops_any}->{unit});
    createPlot($jobID, 'mem_bw', $data->{mem_bw}->{series}, $data->{mem_bw}->{unit});
}
