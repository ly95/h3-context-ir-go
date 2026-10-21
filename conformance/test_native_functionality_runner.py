"""Negative controls for the evidence gate; these do not count as source cases."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from run_native_functionality import primary_result,go_result,load_primary_report,load_compatibility_report,require_compatibility_mappings,COMPATIBILITY_NODES,COMPATIBILITY_EVIDENCE,MODULE,PIN
from compose_licence_replay import source_key

class PrimaryGateTests(unittest.TestCase):
    def report(self):
        return {'collected':[{'nodeid':'original','upstream_status':'passed','phases':[{'phase':'call','outcome':'passed'}]}],
                'go_core_calls':[{'nodeid':'original','function':'primary'}],'validator_calls':[{'nodeid':'original'}]}
    def test_only_the_exact_primary_operation_counts(self):
        case={'upstream_nodeid':'original','required_primary_operations':['primary'],'require_native_validator_calls':True}
        self.assertEqual(primary_result(case,self.report())['status'],'passed')
        report=self.report();report['go_core_calls'][0]['function']='helper'
        self.assertEqual(primary_result(case,report)['status'],'failed')
        report=self.report();report['go_core_calls'][0]['nodeid']='another node'
        self.assertEqual(primary_result(case,report)['status'],'failed')
    def test_required_validator_and_original_assertion_cannot_be_skipped(self):
        case={'upstream_nodeid':'original','required_primary_operations':['primary'],'require_native_validator_calls':True}
        report=self.report();report['validator_calls']=[]
        self.assertEqual(primary_result(case,report)['status'],'failed')
        for outcome in ('failed','skipped',None):
            report=self.report();report['collected'][0]['phases'][0]['outcome']=outcome
            self.assertEqual(primary_result(case,report)['status'],'failed')
    def test_go_root_pass_cannot_stand_in_for_missing_leaf(self):
        case={'go_package':'./compiler','go_test':'Root/exact'}
        self.assertEqual(go_result(case,{('github.com/ly95/contextir/compiler','Root'):'pass'})['status'],'failed')
        self.assertEqual(go_result(case,{('github.com/ly95/contextir/compiler','Root/exact'):'skip'})['status'],'failed')
        self.assertEqual(go_result(case,{('github.com/ly95/contextir/compiler','Root/exact'):'pass'})['status'],'passed')
    def test_modified_source_report_is_rejected_before_native_claim(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);report=root/'report.json';provenance=root/'provenance.json';nodes={str(i)for i in range(991)}
            report.write_text(json.dumps({'source_commit':PIN,'collection_matches_baseline':True,'collected':[{'nodeid':n}for n in nodes]}))
            provenance.write_text(json.dumps({'report_sha256':'wrong'}))
            with self.assertRaisesRegex(ValueError,'report hash differs'):load_primary_report(report,provenance,nodes)
    def test_temp_path_adapter_does_not_hide_other_input_differences(self):
        a={'brief':{'assets':[{'path':'/old/pytest-1/test/plate.png','note':'retained'}]},'cards':{}}
        b={'brief':{'assets':[{'path':'/new/pytest-9/test/plate.png','note':'retained'}]},'cards':{}}
        self.assertEqual(source_key(a),source_key(b))
        b['brief']['assets'][0]['note']='changed'
        self.assertNotEqual(source_key(a),source_key(b))

class CompatibilityGateTests(unittest.TestCase):
    """Gate fixtures are explicit test doubles, not real minimum-Go evidence."""
    def evidence(self,root):
        events=[]
        for name in COMPATIBILITY_NODES.values():
            events.append({'Action':'pass','Package':MODULE,'Test':name})
        events.append({'Action':'pass','Package':MODULE})
        raw=root/'events.jsonl';raw.write_text(''.join(json.dumps(x)+'\n' for x in events))
        from run_native_functionality import digest
        value={'schema':'contextir-go-minimum-compatibility/v1','declared_go_version':'1.25','actual_go_version':'go1.25.0','go_environment':{'GOTOOLCHAIN':'local','GOWORK':'off','GOFLAGS':''},'command':['go','test','-json','-count=1','./...'],'exit_code':0,'passed':True,'source_files_unchanged_during_run':True,'source_sha256':{'go.mod':'fixed'},'raw_go_events':str(raw),'raw_go_events_sha256':digest(raw)}
        report=root/'report.json';report.write_text(json.dumps(value))
        return report,raw,value,events
    def load(self,path):
        with patch('run_native_functionality.declared_go_version',return_value='1.25'),patch('run_native_functionality.compatibility_source_hashes',return_value={'go.mod':'fixed'}):
            return load_compatibility_report(path)
    def test_real_minimum_evidence_is_required_in_addition_to_test_mapping(self):
        mappings={node:{'verification_kind':'go_test','go_package':'./','go_test':test,'required_compatibility_evidence':COMPATIBILITY_EVIDENCE}for node,test in COMPATIBILITY_NODES.items()}
        require_compatibility_mappings(mappings)
        del mappings[next(iter(mappings))]['required_compatibility_evidence']
        with self.assertRaisesRegex(ValueError,'evidence is mandatory'):require_compatibility_mappings(mappings)
        with self.assertRaisesRegex(ValueError,'Missing native language'):require_compatibility_mappings({})
    def test_newer_or_auto_upgrading_toolchain_cannot_stand_in_for_minimum(self):
        with tempfile.TemporaryDirectory() as directory:
            report,raw,value,events=self.evidence(Path(directory));self.assertTrue(self.load(report)['verified'])
            for actual in ('go1.27.2','go1.25.1'):
                value['actual_go_version']=actual;report.write_text(json.dumps(value))
                with self.assertRaisesRegex(ValueError,'exact declared minimum'):self.load(report)
            value['actual_go_version']='go1.25.0';value['go_environment']['GOTOOLCHAIN']='auto';report.write_text(json.dumps(value))
            with self.assertRaisesRegex(ValueError,'disable auto-upgrade'):self.load(report)
    def test_filtered_tests_source_drift_and_changed_events_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            report,raw,value,events=self.evidence(Path(directory))
            value['command'].insert(-1,'-run=^TestGoCompatibility');report.write_text(json.dumps(value))
            with self.assertRaisesRegex(ValueError,'cannot filter'):self.load(report)
            value['command'].remove('-run=^TestGoCompatibility');value['source_sha256']={'go.mod':'stale'};report.write_text(json.dumps(value))
            with self.assertRaisesRegex(ValueError,'current native source'):self.load(report)
            value['source_sha256']={'go.mod':'fixed'};report.write_text(json.dumps(value));raw.write_text(raw.read_text()+'{}\n')
            with self.assertRaisesRegex(ValueError,'raw event hash'):self.load(report)
    def test_minimum_run_must_include_both_tests_without_skips_or_failure(self):
        from run_native_functionality import digest
        with tempfile.TemporaryDirectory() as directory:
            report,raw,value,events=self.evidence(Path(directory))
            for action in ('skip','fail'):
                bad=[dict(x)for x in events];bad[0]['Action']=action;raw.write_text(''.join(json.dumps(x)+'\n'for x in bad));value['raw_go_events_sha256']=digest(raw);report.write_text(json.dumps(value))
                with self.assertRaisesRegex(ValueError,'failure, skipped tests'):self.load(report)
            raw.write_text(''.join(json.dumps(x)+'\n'for x in events[1:]));value['raw_go_events_sha256']=digest(raw);report.write_text(json.dumps(value))
            with self.assertRaisesRegex(ValueError,'omitted compatibility test'):self.load(report)
    def test_no_test_file_package_is_not_a_skipped_test(self):
        from run_native_functionality import digest
        with tempfile.TemporaryDirectory() as directory:
            report,raw,value,events=self.evidence(Path(directory));pkg=MODULE+'/command'
            events.extend([{'Action':'output','Package':pkg,'Output':'? '+pkg+' [no test files]\n'},{'Action':'skip','Package':pkg}])
            raw.write_text(''.join(json.dumps(x)+'\n'for x in events));value['raw_go_events_sha256']=digest(raw);report.write_text(json.dumps(value))
            self.assertTrue(self.load(report)['verified'])
            events[-2]['Output']='';raw.write_text(''.join(json.dumps(x)+'\n'for x in events));value['raw_go_events_sha256']=digest(raw);report.write_text(json.dumps(value))
            with self.assertRaisesRegex(ValueError,'packages did not compile'):self.load(report)
    def test_root_module_slash_mapping_executes_exact_actual_test(self):
        case={'go_package':'./','go_test':'TestGoCompatibilityEverySourceParses'}
        self.assertEqual(go_result(case,{(MODULE,case['go_test']):'pass'})['status'],'passed')

if __name__=='__main__':unittest.main()
