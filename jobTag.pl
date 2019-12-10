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

use DBI;

my $database = 'jobDB';

my %attr = (
    PrintError => 1,
    RaiseError => 1
);

my $dbh = DBI->connect(
    "DBI:SQLite:dbname=$database", "", "", \%attr)
    or die "Could not connect to database: $DBI::errstr";

my $sth_select_tagged_jobs = $dbh->prepare(qq{
    SELECT j.*
    FROM job j
    JOIN jobtag jt ON j.id = jt.job_id
    JOIN tag t ON jt.tag_id = t.id
    WHERE t.name = ?
    });

my $sth_select_job_tags = $dbh->prepare(qq{
    SELECT t.*
    FROM tag t
    JOIN jobtag jt ON t.id = jt.tag_id
    JOIN job j ON jt.job_id = j.id
    WHERE j.job_id = ?
    });

my $sth_select_job = $dbh->prepare(qq{
    SELECT id
    FROM job
    WHERE job_id=?
    });

my $sth_select_tag = $dbh->prepare(qq{
    SELECT id
    FROM tag
    WHERE name=?
    });

my $sth_insert_tag = $dbh->prepare(qq{
    INSERT INTO tag(type,name)
    VALUES(?,?)
    });

my $sth_job_add_tag = $dbh->prepare(qq{
    INSERT INTO jobtag(job_id,tag_id)
    VALUES(?,?)
    });

my $sth_job_has_tag = $dbh->prepare(qq{
    SELECT id FROM job
    WHERE job_id=? AND tag_id=?
    });

my $CMD = $ARGV[0];
my $JOB_ID = $ARGV[1];
my $TAG_NAME = $ARGV[2];

my ($jid, $tid);

# check if job exists
my @row = $dbh->selectrow_array($sth_select_job, undef, $JOB_ID);

if ( @row ) {
    $jid = $row[0];
} else {
    die "Job does not exist: $JOB_ID!\n";
}

# check if tag already exists
@row = $dbh->selectrow_array($sth_select_tag, undef, $TAG_NAME);

if ( @row ) {
    $tid = $row[0];
} else {
    print "Insert new tag: $TAG_NAME!\n";

    $sth_insert_tag->execute('pathologic', $TAG_NAME);
}

if ( $CMD eq 'ADD' ) {
    @row = $dbh->selectrow_array($sth_job_has_tag, undef, $jid, $tid);
    if ( @row ) {
        die "Job already tagged!\n";
    } else {
        print "Adding tag $TAG_NAME to job $JOB_ID!\n";
        $sth_job_add_tag($jid, $tid);
    }
}
elsif ( $CMD eq 'RM' ) {
    # elsif...
}
elsif ( $CMD eq 'LST' ) {
    $sth_select_job_tags->execute;
    my ($id, $type, $name);

    while(($id,$type,$name) = $sth->fetchrow()){
        print("$id, $type, $name\n");
    }
    $sth_select_job_tags->finish();
}
elsif ( $CMD eq 'LSJ' ) {
    # elsif...
}
else {
    die "Unknown command: $CMD!\n";
}

$dbh->disconnect();
