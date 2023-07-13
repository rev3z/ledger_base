// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"github.com/rev3z/ledger_base/leveldb/iterator"
	"github.com/rev3z/ledger_base/leveldb/memdb"
	"github.com/rev3z/ledger_base/leveldb/opt"
	"sync/atomic"
)

const (
	undefinedCompaction = iota
	level0Compaction
	nonLevel0Compaction
	seekCompaction
)

func (s *session) pickMemdbLevel(umin, umax []byte, maxLevel int) int {
	v := s.version()
	defer v.release()
	return v.pickMemdbLevel(umin, umax, maxLevel)
}
func (s *session) pickMemdbLevel_s(umin, umax []byte, maxLevel int) int {
	v := s.version()
	defer v.release()
	return v.pickMemdbLevel_s(umin, umax, maxLevel)
}
func (s *session) flushMemdb_s(rec *sessionRecord, mdb *memdb.DBs, maxLevel int) (int, error) {
	// Create sorted table.
	iter := mdb.NewIterator_s(nil)
	defer iter.Release()
	t, n, err := s.tops.createFrom_s(iter) //这里t是一个sfile
	if err != nil {
		return 0, err
	}
	//Pick level other than zero can cause compaction issue with large
	//bulk insert and delete on strictly incrementing key-space. The
	//problem is that the small deletion markers trapped at lower level,
	//while key/value entries keep growing at higher level. Since the
	//key-space is strictly incrementing it will not overlaps with
	//higher level, thus maximum possible level is always picked, while
	//overlapping deletion marker pushed into lower level.
	// See: https://github.com/syndtr/goleveldb/issues/127.
	flushLevel := s.pickMemdbLevel_s(t.imin.ukey(), t.imax.ukey(), maxLevel)
	rec.addTableFile_s(flushLevel, t)
	//fmt.Println("t.fd:",t.fd,"\n","t.size:",t.size,"\n","t.imax:",t.imax,"\n","t.imin:",t.imin)
	s.logf("memdb@flush created L%d@%d N·%d S·%s %q:%q", flushLevel, t.fd.Num, n, shortenb(int(t.size)), t.imin, t.imax)
	return flushLevel, nil
}
//flushmemdb通过createfrom函数将数据写入磁盘，记录日志并返回当前文件所在的level。
// 在memcompaction操作中，level为0。而createfrom函数的主要功能是创建新的文件，将frozenmemdb中的数据取出，然后刷新到磁盘。
//flushMemdb -> session.tOps.createFrom(得到key之类的信息) -> create(返回一个*tWriter) -> w.finish(写入完成并返回一个tfile)
func (s *session) flushMemdb(rec *sessionRecord, mdb *memdb.DB, maxLevel int) (int, error) {
	// Create sorted table.
	iter := mdb.NewIterator(nil) //immutable的迭代器
	defer iter.Release()
	t, n, err := s.tops.createFrom(iter) //n为 number of entries added so far.
	if err != nil {
		return 0, err
	}
	// Pick level other than zero can cause compaction issue with large
	// bulk insert and delete on strictly incrementing key-space. The
	// problem is that the small deletion markers trapped at lower level,
	// while key/value entries keep growing at higher level. Since the
	// key-space is strictly incrementing it will not overlaps with
	// higher level, thus maximum possible level is always picked, while
	// overlapping deletion marker pushed into lower level.
	// See: https://github.com/syndtr/goleveldb/issues/127.
	flushLevel := s.pickMemdbLevel(t.imin.ukey(), t.imax.ukey(), maxLevel) //当前的level？
	rec.addTableFile(flushLevel, t)

	s.logf("memdb@flush created L%d@%d N·%d S·%s %q:%q", flushLevel, t.fd.Num, n, shortenb(int(t.size)), t.imin, t.imax)
	return flushLevel, nil
}

// Pick a compaction based on current state; need external synchronization.
// 得到触发compaction的类型，并得到初步要参与compaction的数据t0，调用new compaction
func (s *session) pickCompaction() *compaction {
	v := s.version()//获取当前的版本
	//声明三个变量
	var sourceLevel int
	var t0 tFiles //存放某一层的tfile
	var typ int
	if v.cScore >= 1 { //由size触发的，clevel层需要合并
		sourceLevel = v.cLevel
		cptr := s.getCompPtr(sourceLevel) // Get compaction ptr at given level; need external synchronization.
		tables := v.levels[sourceLevel] //某一层的tfile集合？ levels多层的tfiles，tfiles一层的tfile？是这样的逻辑？
		for _, t := range tables { //t是一个tfile
			if cptr == nil || s.icmp.Compare(t.imax, cptr) > 0 {
				t0 = append(t0, t)
				break
			}
		}
		if len(t0) == 0 {
			t0 = append(t0, tables[0])
		}
		if sourceLevel == 0 {
			typ = level0Compaction //minor
		} else {
			typ = nonLevel0Compaction //major
		}
	} else { //由seek触发的
		if p := atomic.LoadPointer(&v.cSeek); p != nil {
			ts := (*tSet)(p)
			sourceLevel = ts.level
			t0 = append(t0, ts.table)
			typ = seekCompaction
		} else {
			v.release()
			return nil
		}
	}

	return newCompaction(s, v, sourceLevel, t0, typ)
}
func (s *session) pickCompaction_s() *compaction {
	v := s.version()//获取当前的版本
	//声明三个变量
	var sourceLevel int
	var t0 sFiles //存放某一层的tfile
	var typ int
	//fmt.Println("进入pickCompaction_s选取合并文件")
	if v.cScores >= 1 { //由size触发的，clevel层需要合并
		sourceLevel = v.cLevels
		cptr := s.getCompPtr_s(sourceLevel) // Get compaction ptr at given level; need external synchronization.
		tables := v.level_s[sourceLevel] //某一层的tfile集合？ levels多层的tfiles，tfiles一层的tfile？是这样的逻辑？
		for _, t := range tables { //t是一个sfile
			if cptr == nil || s.icmp.Compare(t.imax, cptr) > 0 {
				t0 = append(t0, t)
				break
			}
		}
		if len(t0) == 0 {
			t0 = append(t0, tables[0])
		}
		if sourceLevel == 0 {
			typ = level0Compaction //minor
		} else {
			typ = nonLevel0Compaction //major
		}
	} else { //由seek触发的
		if p := atomic.LoadPointer(&v.nSeek); p != nil {
			ts := (*tSet_s)(p)
			sourceLevel = ts.level
			t0 = append(t0, ts.table)
			typ = seekCompaction
		} else {
			v.release()
			return nil
		}
	}

	return newCompaction_s(s, v, sourceLevel, t0, typ) //return c *compare
}
// Create compaction from given level and range; need external synchronization.
// 会在table range compaction中被调用
func (s *session) getCompactionRange(sourceLevel int, umin, umax []byte, noLimit bool) *compaction {
	v := s.version()

	if sourceLevel >= len(v.levels) {
		v.release()
		return nil
	}

	t0 := v.levels[sourceLevel].getOverlaps(nil, s.icmp, umin, umax, sourceLevel == 0)
	if len(t0) == 0 {
		v.release()
		return nil
	}

	//Avoid compacting too much in one shot in case the range is large.
	//But we cannot do this for level-0 since level-0 files can overlap
	//and we must not pick one file and drop another older file if the
	//two files overlap.
	if !noLimit && sourceLevel > 0 {
		limit := int64(v.s.o.GetCompactionSourceLimit(sourceLevel))
		total := int64(0)
		for i, t := range t0 {
			total += t.size
			if total >= limit {
				s.logf("table@compaction limiting F·%d -> F·%d", len(t0), i+1)
				t0 = t0[:i+1]
				break
			}
		}
	}

	typ := level0Compaction
	if sourceLevel != 0 {
		typ = nonLevel0Compaction
	}
	return newCompaction(s, v, sourceLevel, t0, typ)
}
func (s *session) getCompactionRange_s(sourceLevel int, umin, umax []byte, noLimit bool) *compaction {
	v := s.version()

	if sourceLevel >= len(v.level_s) {
		v.release()
		return nil
	}

	t0 := v.level_s[sourceLevel].getOverlaps(nil, s.icmp, umin, umax, sourceLevel == 0)
	if len(t0) == 0 {
		v.release()
		return nil
	}

	//Avoid compacting too much in one shot in case the range is large.
	//But we cannot do this for level-0 since level-0 files can overlap
	//and we must not pick one file and drop another older file if the
	//two files overlap.
	if !noLimit && sourceLevel > 0 {
		limit := int64(v.s.o.GetCompactionSourceLimit(sourceLevel))
		total := int64(0)
		for i, t := range t0 {
			total += t.size
			if total >= limit {
				s.logf("table@compaction limiting F·%d -> F·%d", len(t0), i+1)
				t0 = t0[:i+1]
				break
			}
		}
	}

	typ := level0Compaction
	if sourceLevel != 0 {
		typ = nonLevel0Compaction
	}
	return newCompaction_s(s, v, sourceLevel, t0, typ)
}
//调用expand()
func newCompaction(s *session, v *version, sourceLevel int, t0 tFiles, typ int) *compaction {
	c := &compaction{
		s:             s,
		v:             v,
		typ:           typ,//知道了触发的类型
		sourceLevel:   sourceLevel,//此为参与合并的是哪一层
		levels:        [2]tFiles{t0, nil},//得到了参与compaction的第一层数据
		maxGPOverlaps: int64(s.o.GetCompactionGPOverlaps(sourceLevel)),
		tPtrs:         make([]int, len(v.levels)),//一块空间
	}
	c.expand()
	c.save()
	return c
}
func newCompaction_s(s *session, v *version, sourceLevel int, t0 sFiles, typ int) *compaction {
	c := &compaction{
		s:             s,
		v:             v,
		typ:           typ,//知道了触发的类型
		sourceLevel:   sourceLevel,//此为参与合并的是哪一层
		level_s:        [2]sFiles{t0, nil},//得到了参与compaction的第一层数据
		maxGPOverlaps: int64(s.o.GetCompactionGPOverlaps(sourceLevel)),
		tPtrs:         make([]int, len(v.level_s)),//一块空间
	}
	c.expand_s()
	c.save()
	return c
}

// compaction represent a compaction state.
type compaction struct {
	s *session //会话
	v *version //版本

	typ           int
	sourceLevel   int
	levels        [2]tFiles //两层的sst？，所以用[2]吗，也就是说所有的sst的meta data有tfiles存
	level_s		  [2]sFiles
	maxGPOverlaps int64

	gp                tFiles //第一层?
	gps 			  sFiles //第二层？
	gpi               int
	seenKey           bool
	gpOverlappedBytes int64
	imin, imax        internalKey
	tPtrs             []int
	released          bool
	//快照？
	snapGPI               int
	snapSeenKey           bool
	snapGPOverlappedBytes int64
	snapTPtrs             []int
}

func (c *compaction) save() {
	c.snapGPI = c.gpi
	c.snapSeenKey = c.seenKey
	c.snapGPOverlappedBytes = c.gpOverlappedBytes
	c.snapTPtrs = append(c.snapTPtrs[:0], c.tPtrs...)
}

func (c *compaction) restore() {
	c.gpi = c.snapGPI
	c.seenKey = c.snapSeenKey
	c.gpOverlappedBytes = c.snapGPOverlappedBytes
	c.tPtrs = append(c.tPtrs[:0], c.snapTPtrs...)
}

func (c *compaction) release() {
	if !c.released {
		c.released = true
		c.v.release()
	}
}

// Expand compacted tables; need external synchronization.
func (c *compaction) expand() {
	limit := int64(c.s.o.GetCompactionExpandLimit(c.sourceLevel))//参与compaction的大小限制？
	vt0 := c.v.levels[c.sourceLevel]
	vt1 := tFiles{} //暂且为空
	if level := c.sourceLevel + 1; level < len(c.v.levels) {//下一层
		vt1 = c.v.levels[level]
	}

	t0, t1 := c.levels[0], c.levels[1] //t1=nil
	imin, imax := t0.getRange(c.s.icmp) //得到t0中的最大值和最小值，某一层的最大key和最小key

	// For non-zero levels, the ukey can't hop across tables at all.
	if c.sourceLevel == 0 {
		// We expand t0 here just incase ukey hop across tables.
		t0 = vt0.getOverlaps(t0, c.s.icmp, imin.ukey(), imax.ukey(), c.sourceLevel == 0)//返回与给定的key range有重叠的tables
		if len(t0) != len(c.levels[0]) {
			imin, imax = t0.getRange(c.s.icmp)
		}
	}
	t1 = vt1.getOverlaps(t1, c.s.icmp, imin.ukey(), imax.ukey(), false)
	// Get entire range covered by compaction.
	amin, amax := append(t0, t1...).getRange(c.s.icmp) //返回两层tables的最大key和最小key？

	// See if we can grow the number of inputs in "sourceLevel" without
	// changing the number of "sourceLevel+1" files we pick up.
	if len(t1) > 0 {
		exp0 := vt0.getOverlaps(nil, c.s.icmp, amin.ukey(), amax.ukey(), c.sourceLevel == 0)
		if len(exp0) > len(t0) && t1.size()+exp0.size() < limit {
			xmin, xmax := exp0.getRange(c.s.icmp)
			exp1 := vt1.getOverlaps(nil, c.s.icmp, xmin.ukey(), xmax.ukey(), false)
			if len(exp1) == len(t1) {
				c.s.logf("table@compaction expanding L%d+L%d (F·%d S·%s)+(F·%d S·%s) -> (F·%d S·%s)+(F·%d S·%s)",
					c.sourceLevel, c.sourceLevel+1, len(t0), shortenb(int(t0.size())), len(t1), shortenb(int(t1.size())),
					len(exp0), shortenb(int(exp0.size())), len(exp1), shortenb(int(exp1.size())))
				imin, imax = xmin, xmax
				t0, t1 = exp0, exp1
				amin, amax = append(t0, t1...).getRange(c.s.icmp)
			}
		}
	}

	// Compute the set of grandparent files that overlap this compaction
	// (parent == sourceLevel+1; grandparent == sourceLevel+2)
	if level := c.sourceLevel + 2; level < len(c.v.levels) {
		c.gp = c.v.levels[level].getOverlaps(c.gp, c.s.icmp, amin.ukey(), amax.ukey(), false)
	}

	c.levels[0], c.levels[1] = t0, t1
	c.imin, c.imax = imin, imax
}
func (c *compaction) expand_s() {
	limit := int64(c.s.o.GetCompactionExpandLimit(c.sourceLevel))//参与compaction的大小限制？
	vt0 := c.v.level_s[c.sourceLevel]
	vt1 := sFiles{} //暂且为空
	if level := c.sourceLevel + 1; level < len(c.v.level_s) {//下一层
		vt1 = c.v.level_s[level]
	}

	t0, t1 := c.level_s[0], c.level_s[1] //t1=nil
	imin, imax := t0.getRange(c.s.icmp) //得到t0中的最大值和最小值，某一层的最大key和最小key

	// For non-zero levels, the ukey can't hop across tables at all.
	if c.sourceLevel == 0 {
		// We expand t0 here just incase ukey hop across tables.
		t0 = vt0.getOverlaps(t0, c.s.icmp, imin.ukey(), imax.ukey(), c.sourceLevel == 0)//返回与给定的key range有重叠的tables
		if len(t0) != len(c.level_s[0]) {
			imin, imax = t0.getRange(c.s.icmp)
		}
	}
	t1 = vt1.getOverlaps(t1, c.s.icmp, imin.ukey(), imax.ukey(), false)
	// Get entire range covered by compaction.
	amin, amax := append(t0, t1...).getRange(c.s.icmp) //返回两层tables的最大key和最小key？

	// See if we can grow the number of inputs in "sourceLevel" without
	// changing the number of "sourceLevel+1" files we pick up.
	if len(t1) > 0 {
		exp0 := vt0.getOverlaps(nil, c.s.icmp, amin.ukey(), amax.ukey(), c.sourceLevel == 0)
		if len(exp0) > len(t0) && t1.size()+exp0.size() < limit {
			xmin, xmax := exp0.getRange(c.s.icmp)
			exp1 := vt1.getOverlaps(nil, c.s.icmp, xmin.ukey(), xmax.ukey(), false)
			if len(exp1) == len(t1) {
				c.s.logf("table@compaction expanding L%d+L%d (F·%d S·%s)+(F·%d S·%s) -> (F·%d S·%s)+(F·%d S·%s)",
					c.sourceLevel, c.sourceLevel+1, len(t0), shortenb(int(t0.size())), len(t1), shortenb(int(t1.size())),
					len(exp0), shortenb(int(exp0.size())), len(exp1), shortenb(int(exp1.size())))
				imin, imax = xmin, xmax
				t0, t1 = exp0, exp1
				amin, amax = append(t0, t1...).getRange(c.s.icmp)
			}
		}
	}

	// Compute the set of grandparent files that overlap this compaction
	// (parent == sourceLevel+1; grandparent == sourceLevel+2)
	if level := c.sourceLevel + 2; level < len(c.v.level_s) {
		c.gps = c.v.level_s[level].getOverlaps(c.gps, c.s.icmp, amin.ukey(), amax.ukey(), false)
	}

	c.level_s[0], c.level_s[1] = t0, t1
	c.imin, c.imax = imin, imax
}
// Check whether compaction is trivial.
func (c *compaction) trivial() bool {
	return len(c.levels[0]) == 1 && len(c.levels[1]) == 0 && c.gp.size() <= c.maxGPOverlaps
}
func (c *compaction) trivial_s() bool {
	return len(c.level_s[0]) == 1 && len(c.level_s[1]) == 0 && c.gps.size() <= c.maxGPOverlaps
}
func (c *compaction) baseLevelForKey(ukey []byte) bool {
	for level := c.sourceLevel + 2; level < len(c.v.levels); level++ {
		tables := c.v.levels[level]
		for c.tPtrs[level] < len(tables) {
			t := tables[c.tPtrs[level]]
			if c.s.icmp.uCompare(ukey, t.imax.ukey()) <= 0 {
				// We've advanced far enough.
				if c.s.icmp.uCompare(ukey, t.imin.ukey()) >= 0 {
					// Key falls in this file's range, so definitely not base level.
					return false
				}
				break
			}
			c.tPtrs[level]++
		}
	}
	return true
}
func (c *compaction) baseLevelForKey_s(ukey []byte) bool {
	for level := c.sourceLevel + 2; level < len(c.v.level_s); level++ {
		tables := c.v.level_s[level]
		for c.tPtrs[level] < len(tables) {
			t := tables[c.tPtrs[level]]
			if c.s.icmp.uCompare(ukey, t.imax.ukey()) <= 0 {
				// We've advanced far enough.
				if c.s.icmp.uCompare(ukey, t.imin.ukey()) >= 0 {
					// Key falls in this file's range, so definitely not base level.
					return false
				}
				break
			}
			c.tPtrs[level]++
		}
	}
	return true
}
func (c *compaction) shouldStopBefore(ikey internalKey) bool {
	for ; c.gpi < len(c.gp); c.gpi++ {
		gp := c.gp[c.gpi]
		if c.s.icmp.Compare(ikey, gp.imax) <= 0 {
			break
		}
		if c.seenKey {
			c.gpOverlappedBytes += gp.size
		}
	}
	c.seenKey = true

	if c.gpOverlappedBytes > c.maxGPOverlaps {
		// Too much overlap for current output; start new output.
		c.gpOverlappedBytes = 0
		return true
	}
	return false
}
func (c *compaction) shouldStopBefore_s(ikey internalKey) bool {
	for ; c.gpi < len(c.gps); c.gpi++ {
		gps := c.gps[c.gpi]
		if c.s.icmp.Compare(ikey, gps.imax) <= 0 {
			break
		}
		if c.seenKey {
			c.gpOverlappedBytes += gps.size
		}
	}
	c.seenKey = true

	if c.gpOverlappedBytes > c.maxGPOverlaps {
		// Too much overlap for current output; start new output.
		c.gpOverlappedBytes = 0
		return true
	}
	return false
}
// Creates an iterator.
func (c *compaction) newIterator() iterator.Iterator {
	// Creates iterator slice.
	icap := len(c.levels)
	if c.sourceLevel == 0 {
		// Special case for level-0.
		icap = len(c.levels[0]) + 1
	}
	its := make([]iterator.Iterator, 0, icap)

	// Options.
	ro := &opt.ReadOptions{
		DontFillCache: true,
		Strict:        opt.StrictOverride,
	}
	strict := c.s.o.GetStrict(opt.StrictCompaction)
	if strict {
		ro.Strict |= opt.StrictReader
	}

	for i, tables := range c.levels {
		if len(tables) == 0 {
			continue
		}

		// Level-0 is not sorted and may overlaps each other.
		if c.sourceLevel+i == 0 {
			for _, t := range tables {
				its = append(its, c.s.tops.newIterator(t, nil, ro))
			}
		} else {
			it := iterator.NewIndexedIterator(tables.newIndexIterator(c.s.tops, c.s.icmp, nil, ro), strict)
			its = append(its, it)
		}
	}

	return iterator.NewMergedIterator(its, c.s.icmp, strict)
}
func (c *compaction) newIterator_s() iterator.Iterator {
	// Creates iterator slice.
	icap := len(c.level_s)
	if c.sourceLevel == 0 {
		// Special case for level-0.
		icap = len(c.level_s[0]) + 1
	}
	its := make([]iterator.Iterator, 0, icap)

	// Options.
	ro := &opt.ReadOptions{
		DontFillCache: true,
		Strict:        opt.StrictOverride,
	}
	strict := c.s.o.GetStrict(opt.StrictCompaction)
	if strict {
		ro.Strict |= opt.StrictReader
	}

	for i, tables := range c.level_s {
		if len(tables) == 0 {
			continue
		}

		// Level-0 is not sorted and may overlaps each other.
		if c.sourceLevel+i == 0 {
			for _, t := range tables {
				its = append(its, c.s.tops.newIterator_s(t, nil, ro))
			}
		} else {
			it := iterator.NewIndexedIterator(tables.newIndexIterator(c.s.tops, c.s.icmp, nil, ro), strict)
			its = append(its, it)
		}
	}

	return iterator.NewMergedIterator(its, c.s.icmp, strict)
}
