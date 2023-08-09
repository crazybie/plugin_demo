import argparse
import datetime
import json
import os
import random
import re
import shutil
import subprocess
import time
import traceback

AParser = argparse.ArgumentParser()
AParser.add_argument("--output_dir", "-o", default=".")
AParser.add_argument("--debug", "-d", default="")
AParser.add_argument("--patching_sys_pkg_dir", default="plugin_demo/patching_sys")
AParser.add_argument("--watch_dir", "-w", default=".")
AParser.add_argument("--input_dir", "-i")
app_args = AParser.parse_args()

debug = app_args.debug


# Pattern: patching_sys.GetFactories().RegisterTp((*PlayerObj)(nil))
def scan_types(folder):
    types = []
    files = []
    for i in os.listdir(folder):
        if i.endswith('.go'):
            full = os.path.abspath(os.path.join(folder, i))
            d = open(full).read()
            types += list(re.findall(r'\s+patching_sys\.ApplyPendingPatch\[(.*)\]\(\)', d))
            files.append((full, d))

    print('discovered types:')
    print('------')
    for i in types:
        print(i)
    print('------')
    return files, types


def clone_package(folder, files, ver):
    new_folder = f'{folder}_{ver}'
    shutil.rmtree(new_folder, ignore_errors=True)
    os.makedirs(new_folder)
    new_files = []
    replace = {}
    for full, d in files:
        f2 = os.path.abspath(os.path.join(new_folder, os.path.basename(full)))
        new_files.append((f2, d))
        replace[full] = f2

    overlay = os.path.join(folder, 'overlay.json')
    open(overlay, 'w').write(json.dumps({'Replace': replace}, indent=1))
    if debug:
        print('write overlay', os.path.abspath(overlay))
    return new_folder, new_files, overlay


def replace_package(files):
    pkg = ''
    for full, d in files:
        m = re.search(r'package\s+(\w+)', d)
        if not m:
            raise Exception('file has no package: %s' % full)

        p = m.group(1)
        if pkg and p != pkg:
            raise Exception("multiple packages: %s, %s", pkg, p)
        pkg = p
        if pkg != 'main':
            c = re.sub(r'package %s' % pkg, 'package main', d)
            open(full, 'w').write(c)
    if debug:
        print('replace pkg')
    return pkg


def code_gen(folder, types, ver):
    new_types = ''
    reg_types = ''

    for i in types:
        new_types += f'''    
        
func (s *{i})Ver__() string {{
    return "{ver}"
}}

type SoTmp{ver}_{i} struct {{
    {i}
}}

    '''
        reg_types += f'''
    f.RegisterTp((*SoTmp{ver}_{i})(nil))
    '''

    so_main = f'''
package main

import (
    "{app_args.patching_sys_pkg_dir}"
)

type TmpFactory{ver} struct{{
    patching_sys.SoFactory
}}

var tmpFactory TmpFactory{ver}

func GetFactory() patching_sys.Factory {{
    return &tmpFactory
}}

{new_types}

func (f *TmpFactory{ver}) Init() string {{  
    f.SoFactory.Reset()
    {reg_types}
    return "{ver}"
}}
    '''

    if debug:
        print(so_main)
    f_name = os.path.join(folder, 'so_main.go')
    open(f_name, 'w').write(so_main)
    return f_name


def build_so(folder, overlay, so_output):
    print('building', so_output)
    abs_out_dir = os.path.abspath(app_args.output_dir)
    cwd = os.getcwd()
    os.chdir(folder)
    try:
        s_tm = datetime.datetime.now()
        args = ['go', 'build', '-buildmode=plugin', f'-o={abs_out_dir}/{so_output}', '-tags=gdb',
                '-gcflags=all=-N -l']
        if overlay:
            args.append(f'-overlay={os.path.basename(overlay)}')
        if debug: print(args)
        subprocess.check_call(args)
        print('took', datetime.datetime.now() - s_tm)
    finally:
        os.chdir(cwd)


def cleanup(new_folder, files, main_file, overlay):
    shutil.rmtree(new_folder, ignore_errors=True)
    if main_file:
        os.remove(main_file)
    if overlay:
        os.remove(overlay)
    if debug:
        print('cleaned up')


class DirWatcher:
    def __init__(self, folder, suffix, cb):
        self.times = {}
        self.folder = folder
        self.suffix = suffix
        self.cb = cb
        self.read_times()

    def iter_folder(self):
        for root, dirs, files in os.walk(self.folder):
            for f in files:
                if f.endswith(self.suffix):
                    full = os.path.join(root, f)
                    yield full, os.path.getmtime(full)

    def read_times(self):
        for full, mtime in self.iter_folder():
            self.times[full] = mtime

    def run(self):
        print('watching dir', self.folder)
        while True:
            for full, mtime in self.iter_folder():
                if full not in self.times or mtime > self.times[full]:
                    try:
                        self.cb(full)
                    except:
                        traceback.print_exc()
                    self.read_times()
            time.sleep(1)


def file_changed(f):
    print('----------------------')
    print('file changed', os.path.relpath(f, app_args.watch_dir))
    folder = os.path.dirname(f)
    handle_folder(folder)


def handle_folder(folder):
    ver = f'{random.randint(0, 100000)}'

    files, main_file, new_folder, overlay = [], '', '', ''
    try:
        files, types = scan_types(folder)
        if not types:
            print('no types discovered')
            return

        new_folder, files, overlay = clone_package(folder, files, ver)
        pkg = replace_package(files)
        main_file = code_gen(folder, types, ver)
        build_so(folder, overlay, f'{pkg}_{ver}.so')
    finally:
        cleanup(new_folder, files, main_file, overlay)


if app_args.input_dir:
    handle_folder(app_args.input_dir)
else:
    w = DirWatcher(app_args.watch_dir, ".go", file_changed)
    w.run()
