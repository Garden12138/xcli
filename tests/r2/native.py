#!/usr/bin/env python3
"""Opt-in R2 native acceptance in disposable Work and Git projects.

Uses existing Codex/Cursor logins; never changes accounts or native rules.
Raw streams are retained only in private scratch data to observe interruptions.
Counts every dispatched native invocation, including failures, with a locked
ledger shared by all Task workers. Does not reboot the host or publish anything.
"""
import argparse
import fcntl
import hashlib
import json
import os
import platform
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--binary',required=True)
parser.add_argument('--out',required=True)
parser.add_argument('--execute',action='store_true')
parser.add_argument('--budget',type=int,default=24)
parser.add_argument('--cases',nargs='+',choices=['work-single-stop','work-auto-background','project-single-background','project-auto-owner-loss'])
args=parser.parse_args()
if not args.execute:parser.error('native execution requires --execute')
if not 1<=args.budget<=24:parser.error('budget must be 1..24')
output=Path(args.out).resolve()
if output.exists():parser.error('evidence path exists; choose a new path')
os.umask(0o077)
binary=Path(args.binary).resolve()
root=Path(tempfile.mkdtemp(prefix='xcli-r2-native-')).resolve()
os.chmod(root,0o700)
(root/'private').mkdir(mode=0o700)
ledger=root/'calls.jsonl'
(root/'root-location.txt').write_text(str(root))

env=dict(os.environ,XDG_DATA_HOME=str(root/'data'),XCLI_R2_NATIVE_LEDGER=str(ledger))
report={'kind':'r2_native_acceptance','date':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime()),'platform':platform.platform(),'binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'call_limit':args.budget,'scenarios':[]}

def persist():
    calls=[json.loads(x) for x in ledger.read_text().splitlines()] if ledger.exists() else []
    report['calls']=len(calls);report['calls_by_adapter']={a:sum(x['adapter']==a for x in calls) for a in ('codex','cursor')}
    output.parent.mkdir(parents=True,exist_ok=True)
    temp=output.with_suffix('.tmp')
    temp.write_text(json.dumps(report,indent=2)+'\n');temp.replace(output)

def invoke(args,limit=180):
    p=subprocess.run([str(binary),'--config',str(root/'config.json'),*args],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,timeout=limit)
    label=str(time.time_ns());(root/'private'/f'{label}.stdout').write_text(p.stdout);(root/'private'/f'{label}.stderr').write_text(p.stderr)
    try:t=json.loads(p.stdout)
    except ValueError: raise RuntimeError(f'CLI exit {p.returncode}: {p.stderr[-1200:]}')
    return p.returncode,t

for adapter,command in [('codex','codex'),('cursor','cursor-agent')]:
    actual=subprocess.check_output(['which',command],text=True).strip()
    wrapper=root/f'{adapter}-wrapper'
    wrapper.write_text('#!/usr/bin/env python3\nimport fcntl,json,os,sys,time\n'+
      f'actual={actual!r}\nadapter={adapter!r}\n'+
      'if not any(a in ("--help","--version","-h","-v") for a in sys.argv[1:]):\n'
      ' with open(os.environ["XCLI_R2_NATIVE_LEDGER"],"a+") as f:\n'
      '  fcntl.flock(f,fcntl.LOCK_EX);f.seek(0);count=sum(1 for _ in f)\n'
      f'  if count>={args.budget}: print("R2 native invocation budget exhausted",file=sys.stderr);sys.exit(90)\n'
      '  f.write(json.dumps({"adapter":adapter,"time":time.time(),"pid":os.getpid()})+"\\n");f.flush();os.fsync(f.fileno())\n'
      'os.execv(actual,[actual,*sys.argv[1:]])\n')
    wrapper.chmod(0o700)
cfg={'version':1,'default_agent':'codex','recording':{'output':True},'agents':{
 'codex':{'adapter':'codex','command':str(root/'codex-wrapper'),'args':['-c','model_reasoning_effort="low"']},
 'cursor':{'adapter':'cursor','command':str(root/'cursor-wrapper'),'args':['--trust']},
 'antigravity':{'adapter':'antigravity','command':str(root/'not-enabled-in-this-test')}}}
(root/'config.json').write_text(json.dumps(cfg));(root/'config.json').chmod(0o600)
report['native_versions']={a:subprocess.check_output([c,'--version'],text=True,stderr=subprocess.DEVNULL).strip() for a,c in [('codex','codex'),('cursor','cursor-agent')]}
report['invocation_overrides']={'codex_reasoning_effort':'low','private_raw_output':True,'cursor_trust':'generated temporary cwd only'}
persist()

def saved(id):return json.loads((root/'data/xcli/tasks'/id/'task.json').read_text())
def wait_native_worker(id):
    deadline=time.monotonic()+150
    while time.monotonic()<deadline:
        t=saved(id)
        if t['status'] in ('failed','waiting_user','completed','canceled','paused'):raise RuntimeError(f'Task ended before worker interruption: {t["status"]} {t.get("reason")}')
        for a in t.get('attempts',[]):
            if a['role']=='worker':
                paths=list((root/'data/xcli/runs'/a['run_id']).glob('*.raw.log'))
                # Run output directory is resolved below if implementation uses artifacts/.
                paths+=list((root/'data/xcli').glob(f'**/{a["run_id"]}/*.raw.log'))
                for p in paths:
                    if 'item.started' in p.read_text(errors='replace'):
                        return t,a
        time.sleep(.1)
    raise RuntimeError('native worker session start was not observed')

def wait_finished(id):
    deadline=time.monotonic()+1800
    while time.monotonic()<deadline:
        code,t=invoke(['task','status',id,'--json'])
        if not t.get('owner_active'):
            return t
        time.sleep(.3)
    raise RuntimeError('Task did not finish in 30 minutes')

def run_case(name,project,single,interrupt=None):
    case={'name':name,'kind':'project' if project else 'work','mode':'single' if single else 'auto','interruption':interrupt}
    report['scenarios'].append(case);persist()
    if project:
        source=root/(name+'-source');source.mkdir()
        (source/'go.mod').write_text('module r2fixture\n\ngo 1.26\n')
        (source/'calc.go').write_text('package r2fixture\nfunc Add(a,b int) int { return 0 }\n')
        (source/'.gitignore').write_text('.r2-first-verification\n.r2-inflight\n')
        (source/'calc_test.go').write_text('''package r2fixture
import("os";"testing")
func TestAdd(t *testing.T){if _,err:=os.Stat(".r2-first-verification");os.IsNotExist(err){if err:=os.WriteFile(".r2-first-verification",[]byte("observed"),0600);err!=nil{t.Fatal(err)};t.Fatal("Intentional one-time verification failure: retain this evidence and rerun after a bounded repair.")};if Add(2,5)!=7{t.Fatal("Add(2,5) must be 7")}}
''')
        for args in (['init','-q'],['add','.'],['-c','user.name=R2 Fixture','-c','user.email=r2@example.invalid','commit','-qm','fixture baseline']):subprocess.run(['git','-C',str(source),*args],check=True)
        protected={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in source.iterdir() if p.is_file()}
        goal='Fix Add in calc.go to return a+b. Preserve calc_test.go, go.mod and .gitignore unchanged. Runtime runs tests separately. The first runtime verification deliberately fails once; retain evidence, use one repair if needed, then verify and review current results.'
        args=['--project',str(source),'--verify','["go","test","-count=1","./..."]']
    else:
        inputs=root/(name+'-inputs');inputs.mkdir();facts=inputs/'facts.txt';facts.write_text('North: 12\nSouth: 8\nTotal: 20\n')
        goal='Create report.md from the selected input facts. Include literal "Total: 20", North 12 and South 8, and cite the input facts.txt using its absolute path or a correct relative path inside report.md. Do not add unrelated requirements. Runtime performs validation separately.'
        args=['--input',str(facts)]
    if interrupt:goal+=' Before the main work, if the progress marker does not yet exist, use a shell command to write '+('.r2-inflight' if project else 'progress.txt')+' containing started, then sleep 8 seconds. This intentional pause is part of this recovery test. The marker is not an acceptance artifact. If the marker already exists, skip the pause and finish the task.'
    if not single:goal+=' Require review by another CLI, with Codex producing the result and Cursor reviewing it.'
    if single:args+=['--agent','codex']
    code,initial=invoke(['task',goal,*args,'--timeout','30m','--max-calls','8','--detach','--json'])
    if code:raise RuntimeError(initial.get('reason','launch failed'))
    case['task_id']=initial['id'];case['initial_pid']=initial['pid'];persist()
    if interrupt:
        try:
            live,a=wait_native_worker(initial['id'])
        except RuntimeError:
            blocked=saved(initial['id'])
            if blocked['status']!='waiting_user' or blocked.get('reason')!='complete requires work and review':raise
            case['rejected_plan_retained']=True
            clarification='The previous coordinator response lacked work and review and was rejected. Preserve the original goal. Return schema version 1, delegate worker codex, verify argv ["go","test","-count=1","./..."] cwd ".", delegate reviewer cursor, then complete. Acceptance is calc.go with contains:[] and sources:[]. The ignored timing marker is not an acceptance artifact. Preserve the test files.'
            code,resumed=invoke(['task','continue',initial['id'],clarification,'--fresh-session','--detach','--json'],30)
            if code:raise RuntimeError(resumed.get('reason','continuation failed'))
            live,a=wait_native_worker(initial['id'])
        case['interrupted_role']='worker';case['calls_before_interruption']=live['calls']
        if interrupt=='stop':
            code,paused=invoke(['task','stop',initial['id'],'--json'],30)
            case['stop_status']=paused['status'];assert code==0 and paused['status']=='paused'
            code,result=invoke(['task','continue',initial['id'],'--detach','--json'],30)
            assert code==0,(code,result.get('reason'))
        else:
            os.kill(live['pid'],signal.SIGKILL)
            time.sleep(3)
            code,lost=invoke(['task','status',initial['id'],'--json'])
            case['lost_status']=lost['status'];assert lost['status']=='needs_reconcile'
            code,blocked=invoke(['task','continue',initial['id'],'--json'],30)
            case['unsafe_continue_blocked']=code!=0 and blocked['status']=='needs_reconcile';assert case['unsafe_continue_blocked']
            code,result=invoke(['task','continue',initial['id'],'--reconcile','--fresh-session','--detach','--json'],30)
            assert code==0,(code,result.get('reason'))
    result=wait_finished(initial['id'])
    case.update(status=result['status'],reason=result.get('reason'),calls=result['calls'],repairs=result['repairs'],plan_versions=len(result.get('plans',[])),attempt_roles=[x['role'] for x in result.get('attempts',[])],mode_preserved=result['mode']==case['mode'] and (not single or result['agent']=='codex'),same_task=result['id']==initial['id'],budget_seconds=result['options']['Timeout']/1e9,elapsed_seconds=result['elapsed_ns']/1e9)
    runs=[json.loads((root/'data/xcli/runs'/(a['run_id']+'.json')).read_text()) for a in result.get('attempts',[])]
    case['native_resume_attempted']=any(x.get('resumed_from') for x in runs)
    case['native_resume_succeeded']=any(x.get('resumed_from') and x['status']=='success' for x in runs)
    case['run_statuses']=[{'role':r.get('role'),'agent':r.get('agent'),'status':r['status'],'parse_status':r.get('parse_status'),'has_session':bool(r.get('session_id'))} for r in runs]
    case['review_kinds']=[r['kind'] for r in result.get('reviews',[])]
    case['historical_failures_retained']=any(c['status']=='failed' for c in result.get('checks',[])) if project else True
    case['current_review_bound']=bool(result.get('reviews')) and result['reviews'][-1]['snapshot']==result['snapshot'] and result['reviews'][-1]['passed']
    if project:
        case['original_unchanged']=all(hashlib.sha256((source/name).read_bytes()).hexdigest()==h for name,h in protected.items()) and not subprocess.check_output(['git','-C',str(source),'status','--porcelain'],text=True).strip()
        cwd=Path(result['cwd']);case['protected_files_unchanged']=all(hashlib.sha256((cwd/name).read_bytes()).hexdigest()==h for name,h in protected.items() if name!='calc.go')
    else:
        artifact=Path(result['output'])/'report.md'
        case['deliverable_correct']=artifact.exists() and 'Total: 20' in artifact.read_text()
    if result['status']!='completed':
        case['acceptance']='failed';persist();raise RuntimeError(case['reason'])
    code,again=invoke(['task','continue',initial['id'],'--json'],30)
    case['repeat_continue_no_calls']=again['calls']==result['calls']
    events=[json.loads(x) for x in (root/'data/xcli/tasks'/initial['id']/'events.jsonl').read_text().splitlines()]
    case['material_session_fallback']=any(e['type']=='session_unavailable' for e in events)
    case['events_monotonic']=all(x['sequence']==i+1 for i,x in enumerate(events))
    case['acceptance']='passed' if result['status']=='completed' and case['mode_preserved'] and case['same_task'] and case['current_review_bound'] and case['repeat_continue_no_calls'] and case['events_monotonic'] and (case.get('original_unchanged',True)) and case.get('protected_files_unchanged',True) and case.get('deliverable_correct',True) and (single or 'cross_cli' in case['review_kinds']) else 'failed'
    persist();print(json.dumps(case),flush=True)
    if case['acceptance']!='passed':raise RuntimeError(f'{name} did not pass')

try:
    for case in [('work-single-stop',False,True,'stop'),('work-auto-background',False,False,None),('project-single-background',True,True,None),('project-auto-owner-loss',True,False,'kill')]:
        if not args.cases or case[0] in args.cases:run_case(*case)
    report['acceptance']='passed'
except Exception as e:
    report['acceptance']='failed';report['error']=str(e);print(str(e),flush=True)
finally:
    persist();print('PRIVATE_ROOT='+str(root),flush=True)
raise SystemExit(0 if report['acceptance']=='passed' else 1)
