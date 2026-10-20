"""Negative controls for the evidence gate; these do not count as source cases."""
import json
from pathlib import Path
import tempfile
import unittest

from run_native_functionality import primary_result,go_result,load_primary_report,PIN
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

if __name__=='__main__':unittest.main()
